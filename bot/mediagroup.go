package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const mediaGroupProcessDelay = 2 * time.Second // Delay before processing a media group
const maxMediaGroupSize = 100                  // Limit messages per group
const maxSendRetries = 3                       // Retries for sending media group

// storeMessageInGroup adds a message to a temporary storage for its media group.
// It ensures thread-safety using sync.Map and sorts messages by ID.
// Limits the number of messages per group to prevent memory issues.
func (b *Bot) storeMessageInGroup(message telego.Message) (isFirst bool) {
	if message.MediaGroupID == "" {
		return false // Not part of a media group
	}

	val, loaded := b.mediaGroups.LoadOrStore(message.MediaGroupID, []telego.Message{message})
	if !loaded {
		// This was the first message stored for this groupID
		if b.debug {
			log.Printf("[MediaGroup Store Group:%s] Stored first message (ID: %d)", message.MediaGroupID, message.MessageID)
		}
		return true
	}

	// Group already exists, append the message
	msgs := val.([]telego.Message)
	if len(msgs) >= maxMediaGroupSize {
		log.Printf("[MediaGroup Store Group:%s] Group limit reached (%d), dropping message %d", message.MediaGroupID, maxMediaGroupSize, message.MessageID)
		return false // Indicate not the first, although it wasn't stored
	}

	// Append and sort
	msgs = append(msgs, message)
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].MessageID < msgs[j].MessageID
	})
	b.mediaGroups.Store(message.MediaGroupID, msgs)
	if b.debug {
		log.Printf("[MediaGroup Store Group:%s] Appended message (ID: %d). Total: %d", message.MediaGroupID, message.MessageID, len(msgs))
	}
	return false // Not the first message stored
}

// createInputMedia converts a slice of telego.Message (belonging to a media group)
// into a slice of telego.InputMedia suitable for bot.SendMediaGroup.
// It applies the provided caption to the first photo in the group.
func (b *Bot) createInputMedia(msgs []telego.Message, caption string) []telego.InputMedia {
	inputMedia := make([]telego.InputMedia, 0, len(msgs))
	for i, msg := range msgs {
		if msg.Photo != nil && len(msg.Photo) > 0 {
			// Find the largest photo (best quality)
			photo := msg.Photo[0]
			for _, p := range msg.Photo {
				if p.FileSize > photo.FileSize {
					photo = p
				}
			}

			mediaPhoto := &telego.InputMediaPhoto{
				Type:  telego.MediaTypePhoto,
				Media: telego.InputFile{FileID: photo.FileID},
			}
			// Apply caption only to the first element
			if i == 0 && caption != "" {
				mediaPhoto.Caption = caption
				// Consider adding ParseMode if needed, e.g., ModeHTML
				// mediaPhoto.ParseMode = telego.ModeHTML
			}
			inputMedia = append(inputMedia, mediaPhoto)
		} else if msg.Video != nil {
			mediaVideo := &telego.InputMediaVideo{
				Type:  telego.MediaTypeVideo,
				Media: telego.InputFile{FileID: msg.Video.FileID},
				// Add other video fields if needed (thumb, dimensions, duration, etc.)
			}
			if i == 0 && caption != "" {
				mediaVideo.Caption = caption
				// mediaVideo.ParseMode = telego.ModeHTML
			}
			inputMedia = append(inputMedia, mediaVideo)
		} else {
			log.Printf("[createInputMedia] Unsupported message type in media group: MsgID=%d", msg.MessageID)
		}
	}
	return inputMedia
}

// parseRetryAfter attempts to extract the 'retry after N' duration (in seconds)
// from a Telegram API error string (typically for 429 Too Many Requests).
// Returns the duration and true if successful, otherwise 0 and false.
func parseRetryAfter(errorString string) (int, bool) {
	var retryAfter int
	// Example error: "telego: sendMediaGroup: api: 429 Too Many Requests: retry after 5"
	fields := strings.Fields(errorString)
	if len(fields) >= 3 && fields[len(fields)-2] == "after" {
		_, err := fmt.Sscan(fields[len(fields)-1], &retryAfter)
		if err == nil && retryAfter > 0 {
			return retryAfter, true
		}
	}
	// Fallback parsing attempt
	if _, err := fmt.Sscan(errorString, "telego: sendMediaGroup: api: 429 Too Many Requests: retry after %d", &retryAfter); err == nil && retryAfter > 0 {
		return retryAfter, true
	}

	return 0, false
}

// sendMediaGroupWithRetry attempts to send a media group to the configured channel,
// handling potential '429 Too Many Requests' errors by retrying after the specified delay.
// It uses the bot instance (b.bot) and handler (b.handler) for configuration and API calls.
func (b *Bot) sendMediaGroupWithRetry(ctx context.Context, groupID string, inputMedia []telego.InputMedia, maxRetries int) ([]telego.Message, error) {
	var lastErr error
	var retryCount int
	channelID := b.handler.GetChannelID()
	const defaultRetryWait = 2 * time.Second

	logPrefix := fmt.Sprintf("[SendRetry Group:%s]", groupID) // Use actual groupID

	for retryCount < maxRetries {
		sentMessages, err := b.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(channelID),
			Media:  inputMedia,
		})

		if err == nil {
			if b.debug || retryCount > 0 {
				log.Printf("%s Successfully sent after %d attempt(s)", logPrefix, retryCount+1)
			}
			return sentMessages, nil
		}

		lastErr = err
		errStr := err.Error()

		if strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "429") {
			retryAfterSeconds, ok := parseRetryAfter(errStr)
			waitDuration := defaultRetryWait
			if ok {
				log.Printf("%s Rate limit hit (attempt %d/%d), waiting %d seconds", logPrefix, retryCount+1, maxRetries, retryAfterSeconds)
				waitDuration = time.Duration(retryAfterSeconds) * time.Second
			} else {
				log.Printf("%s Rate limit hit (attempt %d/%d), couldn't parse retry time, waiting %v. Error: %s", logPrefix, retryCount+1, maxRetries, defaultRetryWait, errStr)
			}

			select {
			case <-ctx.Done():
				finalErr := fmt.Errorf("%s context cancelled during rate limit wait (attempt %d/%d): %w", logPrefix, retryCount+1, maxRetries, ctx.Err())
				sentry.CaptureException(finalErr)
				return nil, finalErr
			case <-time.After(waitDuration):
				retryCount++
				continue // Continue to the next retry attempt
			}
		}

		// If it was not a rate limit error, wrap and return immediately
		finalErr := fmt.Errorf("%s failed to send media group (attempt %d/%d): %w", logPrefix, retryCount+1, maxRetries, err)
		sentry.CaptureException(finalErr)
		return nil, finalErr
	}

	// If loop finished, max retries were exceeded
	finalErr := fmt.Errorf("%s max retries (%d) exceeded for sending media group: %w", logPrefix, maxRetries, lastErr)
	sentry.CaptureException(finalErr)
	return nil, finalErr
}

// processMediaGroup handles the complete lifecycle of sending a collected media group.
// It retrieves captions, creates InputMedia, sends the group (with retries),
// cleans up temporary storage, and generates a PostLog entry on success.
// Returns the PostLog entry or an error.
func (b *Bot) processMediaGroup(ctx context.Context, groupID string, msgs []telego.Message) (*models.PostLog, error) {
	if len(msgs) == 0 {
		log.Printf("[ProcessMediaGroup Group:%s] Attempted to process empty group.", groupID)
		return nil, nil // Nothing to process
	}
	firstMessage := msgs[0] // Use the first message for context like sender ID
	logPrefix := fmt.Sprintf("[ProcessMediaGroup Group:%s User:%d]", groupID, firstMessage.From.ID)
	log.Printf("%s Processing %d messages.", logPrefix, len(msgs))

	// Retrieve caption (check group-specific first, then user's active caption)
	caption := b.handler.RetrieveMediaGroupCaption(groupID)
	if caption == "" {
		if userCaption, ok := b.handler.GetActiveCaption(firstMessage.Chat.ID); ok {
			caption = userCaption
			log.Printf("%s Using active user caption for group.", logPrefix)
		}
	}

	inputMedia := b.createInputMedia(msgs, caption)
	log.Printf("%s Created %d input media items.", logPrefix, len(inputMedia))

	if len(inputMedia) == 0 {
		log.Printf("%s No valid media found in messages. Skipping send.", logPrefix)
		return nil, nil // Not an error, just nothing to send
	}

	sentMessages, err := b.sendMediaGroupWithRetry(ctx, groupID, inputMedia, maxSendRetries)
	if err != nil {
		log.Printf("%s Error sending group: %v", logPrefix, err)
		return nil, fmt.Errorf("failed to send media group %s: %w", groupID, err)
	}

	publishedTime := time.Now()
	channelPostID := 0
	if len(sentMessages) > 0 {
		channelPostID = sentMessages[0].MessageID // ID of the first message in the sent group
	}

	logEntry := &models.PostLog{
		SenderID:             firstMessage.From.ID,
		SenderUsername:       firstMessage.From.Username,
		Caption:              caption,
		MessageType:          "media_group",
		ReceivedAt:           time.Unix(int64(firstMessage.Date), 0), // Time of the first message received
		PublishedAt:          publishedTime,
		ChannelID:            b.handler.GetChannelID(),
		ChannelPostID:        channelPostID,
		OriginalMediaGroupID: groupID,
	}

	log.Printf("%s Successfully sent group -> Channel Post ID: %d.", logPrefix, channelPostID)
	return logEntry, nil
}

// handleLoadedMediaGroup is the core logic executed after a delay for a potential media group.
// It ensures only the goroutine corresponding to the first message processes the group,
// calls processMediaGroup, logs results, and handles cleanup.
func (b *Bot) handleLoadedMediaGroup(ctx context.Context, groupID string, firstMessageID int) {
	logPrefix := fmt.Sprintf("[HandleLoaded Group:%s]", groupID)

	// Load the final list of messages for the group
	val, ok := b.mediaGroups.Load(groupID)
	if !ok {
		log.Printf("%s Group already processed or cleaned up.", logPrefix)
		return // Already handled or timed out elsewhere
	}
	msgs := val.([]telego.Message)

	if len(msgs) == 0 {
		log.Printf("%s Group is empty, cleaning up.", logPrefix)
		b.mediaGroups.Delete(groupID)
		b.handler.DeleteMediaGroupCaption(groupID)
		return
	}

	// Crucial check: Only proceed if this goroutine was triggered by the *first* message stored for this group.
	if msgs[0].MessageID != firstMessageID {
		if b.debug {
			log.Printf("%s Skipping processing: Not the first message's timer (Expected: %d, Got: %d).", logPrefix, msgs[0].MessageID, firstMessageID)
		}
		return // Let the correct goroutine handle it and the cleanup.
	}

	log.Printf("%s Timer fired for first message (%d). Processing %d messages.", logPrefix, firstMessageID, len(msgs))

	// Corrected call: Pass groupID and msgs slice
	logEntry, err := b.processMediaGroup(ctx, groupID, msgs)

	// --- Cleanup after processing (successful or not) --- Must happen only once per group!
	b.mediaGroups.Delete(groupID)
	b.handler.DeleteMediaGroupCaption(groupID)
	log.Printf("%s Cleaned up group storage.", logPrefix)
	// --- End Cleanup ---

	if err != nil {
		log.Printf("%s Error during media group processing: %v", logPrefix, err)
		// Error already logged/sent to Sentry by processMediaGroup or sendMediaGroupWithRetry
		// Optionally, notify the sender?
		// _, sendErr := b.bot.SendMessage(ctx, tu.Message(tu.ID(msgs[0].Chat.ID), fmt.Sprintf("Failed to process media group: %v", err)))
		// if sendErr != nil { ... }
		return
	}

	if logEntry == nil {
		log.Printf("%s Processing finished, but no log entry generated (e.g., no valid media).", logPrefix)
		return
	}

	// Log the successful post to the database
	if postLogErr := b.handler.LogPublishedPost(*logEntry); postLogErr != nil {
		log.Printf("%s Failed to log media group post to DB: %v", logPrefix, postLogErr)
		// LogPublishedPost logs internally, just capture for Sentry
		sentry.CaptureException(fmt.Errorf("%s failed to log media group post: %w", logPrefix, postLogErr))
	}

	// Update user info after successful processing
	if userUpdateErr := b.handler.UserRepo().UpdateUser(ctx, logEntry.SenderID, logEntry.SenderUsername, msgs[0].From.FirstName, msgs[0].From.LastName, false, "send_media_group"); userUpdateErr != nil {
		log.Printf("%s Failed to update user info after sending media group: %v", logPrefix, userUpdateErr)
		sentry.CaptureException(fmt.Errorf("%s failed to update user after media group: %w", logPrefix, userUpdateErr))
	}

	// Log the user action using the new accessor method
	if actionLogErr := b.handler.ActionLogger().LogUserAction(logEntry.SenderID, "send_media_group", map[string]interface{}{
		"chat_id":            msgs[0].Chat.ID,
		"media_group_id":     groupID,
		"message_count":      len(msgs),
		"channel_message_id": logEntry.ChannelPostID,
	}); actionLogErr != nil {
		log.Printf("%s Failed to log send_media_group action: %v", logPrefix, actionLogErr)
		sentry.CaptureException(fmt.Errorf("%s failed to log send_media_group action: %w", logPrefix, actionLogErr))
	}

	// Send confirmation to the user using localized string
	lang := locales.DefaultLanguage // Default language
	if msgs[0].From != nil && msgs[0].From.LanguageCode != "" {
		// lang = msgs[0].From.LanguageCode // TODO: Use user language
	}
	localizer := locales.NewLocalizer(lang)
	successMsg := locales.GetMessage(localizer, "MsgMediaGroupSuccess", nil, nil)

	if _, successMsgErr := b.bot.SendMessage(ctx, tu.Message(tu.ID(msgs[0].Chat.ID), successMsg)); successMsgErr != nil {
		log.Printf("%s Failed to send success confirmation for media group: %v", logPrefix, successMsgErr)
	}
}

// processMediaGroupAfterDelay waits for a short duration before attempting to process a media group.
// This allows time for subsequent messages in the same group to arrive.
// It loads the group from storage and calls handleLoadedMediaGroup.
func (b *Bot) processMediaGroupAfterDelay(groupID string, firstMessageID int) {
	// Create a new context for the delayed execution, as the original handler context might have expired.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Example timeout
	defer cancel()

	timer := time.NewTimer(mediaGroupProcessDelay)
	defer timer.Stop() // Ensure timer is stopped if function exits early

	log.Printf("[Delay Timer Group:%s] Started %v timer for first message %d.", groupID, mediaGroupProcessDelay, firstMessageID)

	select {
	case <-timer.C:
		log.Printf("[Delay Timer Group:%s] Timer finished, proceeding to handle loaded group.", groupID)
		// Call the handler logic in a separate goroutine to avoid blocking potential future timers?
		// Or handle directly if processing is expected to be fast.
		// For simplicity, handle directly here.
		b.handleLoadedMediaGroup(ctx, groupID, firstMessageID)
	case <-ctx.Done():
		log.Printf("[Delay Timer Group:%s] Context cancelled before timer finished: %v", groupID, ctx.Err())
		// Clean up the group as it won't be processed
		b.mediaGroups.Delete(groupID)
		b.handler.DeleteMediaGroupCaption(groupID)
	}
}

// handleMediaGroupUpdate is the entry point for handling messages that are part of a media group.
// It calls handleMediaGroup to manage storage and delayed processing.
// Note: The context from the original update is not passed down currently.
func (b *Bot) handleMediaGroupUpdate(message telego.Message) {
	if message.MediaGroupID == "" {
		return // Should not happen if called correctly
	}

	// Store the message and check if it's the first one for this group
	isFirst := b.storeMessageInGroup(message)

	// If it's the first message, start the delayed processing timer in a new goroutine.
	if isFirst {
		go b.processMediaGroupAfterDelay(message.MediaGroupID, message.MessageID)
	}
}
