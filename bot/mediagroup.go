package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"vrcmemes-bot/internal/database/models"

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
	const defaultRetryWait = 2 * time.Second

	logPrefix := fmt.Sprintf("[SendRetry Group:%s]", groupID) // Use actual groupID

	for retryCount < maxRetries {
		sentMessages, err := b.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(b.channelID), // Use stored channelID
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
				// Warning: Parsing the error string might be brittle if the API error format changes.
				// Consider checking if telego provides a structured way to get RetryAfter parameter.
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
	caption := b.captionProv.RetrieveMediaGroupCaption(groupID)
	if caption == "" {
		if userCaption, ok := b.captionProv.GetActiveCaption(firstMessage.Chat.ID); ok {
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
		ChannelID:            b.channelID,
		ChannelPostID:        channelPostID,
		OriginalMediaGroupID: groupID,
	}

	// Note: Logging user action and updating user info should be handled
	// by the caller or a dedicated post-processing step if needed,
	// as Bot no longer has direct access to ActionLogger/UserRepo here.
	// The confirmation message to the user should also be handled elsewhere.

	log.Printf("%s Successfully sent group -> Channel Post ID: %d.", logPrefix, channelPostID)
	return logEntry, nil
}

// handleLoadedMediaGroup is the core logic executed after a delay for a potential media group.
// It uses LoadAndDelete for atomicity, ensuring only one goroutine processes the group.
// It then calls processMediaGroup, logs results, and handles cleanup.
func (b *Bot) handleLoadedMediaGroup(ctx context.Context, groupID string) {
	logPrefix := fmt.Sprintf("[HandleLoaded Group:%s]", groupID)

	// Atomically load and delete the group. Only one goroutine will succeed.
	val, loaded := b.mediaGroups.LoadAndDelete(groupID)
	if !loaded {
		// If !loaded is true, it means the key didn't exist (already processed or never existed).
		if b.debug {
			log.Printf("%s Group not found or already processed/deleted.", logPrefix)
		}
		return
	}

	// If loaded is true, 'val' contains the messages and this goroutine has exclusive access.
	msgs, ok := val.([]telego.Message)
	if !ok || len(msgs) == 0 {
		log.Printf("%s Loaded empty or invalid group data after delete. Type: %T", logPrefix, val)
		sentry.CaptureException(fmt.Errorf("%s loaded invalid group data type %T for group %s", logPrefix, val, groupID))
		return
	}

	log.Printf("%s Atomically loaded %d messages for processing.", logPrefix, len(msgs))

	// Process the retrieved media group
	postLogEntry, err := b.processMediaGroup(ctx, groupID, msgs)
	if err != nil {
		// Error during processing (e.g., sending failed after retries)
		log.Printf("%s Error processing media group: %v", logPrefix, err)
		// Error is already captured by sentry inside processMediaGroup or sendMediaGroupWithRetry
		return // Cleanup already happened via LoadAndDelete
	}

	// Log successful post if entry was created
	if postLogEntry != nil {
		err = b.postLogger.LogPublishedPost(*postLogEntry) // Use the post logger
		if err != nil {
			log.Printf("%s Error logging post to database: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s failed to log post: %w", logPrefix, err))
		} else {
			if b.debug {
				log.Printf("%s Post logged successfully.", logPrefix)
			}
		}
	}

	// Cleanup (removing the timer) is implicitly handled because this function
	// is called by the timer itself, and the group is removed by LoadAndDelete.
	// No need to explicitly remove the group from b.mediaGroups here.
	if b.debug {
		log.Printf("%s Finished handling group.", logPrefix)
	}
}

// handleMediaGroupUpdate receives a message belonging to a media group.
// It stores the message and schedules processMediaGroup using time.AfterFunc
// only if this is the *first* message received for the group.
func (b *Bot) handleMediaGroupUpdate(message telego.Message) {
	if b.storeMessageInGroup(message) {
		// If storeMessageInGroup returned true, this is the first message.
		// Schedule the processing function to run after a delay.
		groupID := message.MediaGroupID
		if b.debug {
			log.Printf("[MediaGroup Scheduler Group:%s] First message (ID: %d) received. Scheduling processing in %v.", groupID, message.MessageID, mediaGroupProcessDelay)
		}

		// Use time.AfterFunc to schedule the call without blocking.
		// The context passed should ideally be derived from the application's main context
		// to allow cancellation on shutdown, but managing individual timer contexts can be complex.
		// For now, we use context.Background(), but consider refining context propagation if needed.
		time.AfterFunc(mediaGroupProcessDelay, func() {
			// We pass a background context here. If shutdown needs to interrupt processing,
			// more sophisticated context management would be required (e.g., storing cancel funcs).
			b.handleLoadedMediaGroup(context.Background(), groupID)
		})
	}
}
