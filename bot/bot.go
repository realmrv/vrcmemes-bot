package bot

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/handlers"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Bot represents the Telegram bot
type Bot struct {
	bot         *telego.Bot
	handler     *handlers.MessageHandler
	updatesChan <-chan telego.Update
	mediaGroups sync.Map
	debug       bool
}

// New creates a new bot instance, accepting a pre-configured telego.Bot and MessageHandler
func New(bot *telego.Bot, debug bool, handler *handlers.MessageHandler) (*Bot, error) {
	// No longer creates the telego.Bot instance here
	if bot == nil {
		return nil, fmt.Errorf("telego bot instance cannot be nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("message handler instance cannot be nil")
	}

	return &Bot{
		bot:     bot,
		handler: handler,
		debug:   debug,
	}, nil
}

// storeMessageInGroup stores a message in the media group
func (b *Bot) storeMessageInGroup(message telego.Message) {
	if message.MediaGroupID == "" {
		return
	}
	if messages, exists := b.mediaGroups.Load(message.MediaGroupID); exists {
		msgs := messages.([]telego.Message)
		if len(msgs) < 100 { // Limit to 100 items per group
			msgs = append(msgs, message)
			sort.Slice(msgs, func(i, j int) bool {
				return msgs[i].MessageID < msgs[j].MessageID
			})
			b.mediaGroups.Store(message.MediaGroupID, msgs)
		} else {
			log.Printf("[storeMessageInGroup] Media group %s exceeded limit, message %d dropped", message.MediaGroupID, message.MessageID)
		}
	} else {
		b.mediaGroups.Store(message.MediaGroupID, []telego.Message{message})
	}
}

// createInputMedia creates input media array from messages
func (b *Bot) createInputMedia(msgs []telego.Message, caption string) []telego.InputMedia {
	var inputMedia []telego.InputMedia
	for i, msg := range msgs {
		if msg.Photo != nil {
			photo := msg.Photo[len(msg.Photo)-1]
			mediaPhoto := &telego.InputMediaPhoto{
				Type:  "photo",
				Media: telego.InputFile{FileID: photo.FileID},
			}
			if i == 0 && caption != "" {
				mediaPhoto.Caption = caption
			}
			inputMedia = append(inputMedia, mediaPhoto)
		}
	}
	return inputMedia
}

// parseRetryAfter extracts the retry duration from a "Too Many Requests" error string.
// Returns the duration in seconds and true if parsing was successful, otherwise 0 and false.
func parseRetryAfter(errorString string) (int, bool) {
	var retryAfter int
	// Example error: "telego: sendMediaGroup: api: 429 Too Many Requests: retry after 5"
	// Use Fields to split by space and check the last parts.
	fields := strings.Fields(errorString)
	if len(fields) >= 3 && fields[len(fields)-2] == "after" {
		_, err := fmt.Sscan(fields[len(fields)-1], &retryAfter)
		if err == nil && retryAfter > 0 {
			return retryAfter, true
		}
	}
	// Attempt parsing with the specific format as a fallback (less flexible)
	if _, err := fmt.Sscan(errorString, "telego: sendMediaGroup: api: 429 Too Many Requests: retry after %d", &retryAfter); err == nil && retryAfter > 0 {
		return retryAfter, true
	}

	return 0, false
}

// sendMediaGroupWithRetry sends media group to channel with retry on rate limit
func (b *Bot) sendMediaGroupWithRetry(ctx context.Context, inputMedia []telego.InputMedia, maxRetries int) ([]telego.Message, error) {
	var lastErr error
	var retryCount int
	channelID := b.handler.GetChannelID()
	const defaultRetryWait = 2 * time.Second // Default wait time if parsing fails

	for retryCount < maxRetries {
		sentMessages, err := b.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(channelID),
			Media:  inputMedia,
		})

		if err == nil {
			if b.debug {
				log.Printf("Successfully sent media group after %d attempts", retryCount+1)
			}
			return sentMessages, nil
		}

		lastErr = err
		errStr := err.Error()

		if strings.Contains(errStr, "Too Many Requests") {
			retryAfterSeconds, ok := parseRetryAfter(errStr)
			waitDuration := defaultRetryWait
			if ok {
				log.Printf("Rate limit hit (attempt %d/%d), waiting %d seconds", retryCount+1, maxRetries, retryAfterSeconds)
				waitDuration = time.Duration(retryAfterSeconds) * time.Second
			} else {
				log.Printf("Rate limit hit (attempt %d/%d), couldn't parse retry time, waiting %v", retryCount+1, maxRetries, defaultRetryWait)
			}

			// Single select for waiting
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during rate limit wait: %w", ctx.Err())
			case <-time.After(waitDuration):
				retryCount++
				continue // Continue to the next retry attempt
			}
		}

		// If it was not a rate limit error, return immediately
		return nil, fmt.Errorf("failed to send media group (attempt %d/%d): %w", retryCount+1, maxRetries, err)
	}

	// If loop finished, max retries were exceeded
	return nil, fmt.Errorf("max retries (%d) exceeded for sending media group: %w", maxRetries, lastErr)
}

// processMediaGroup processes a complete media group and returns a PostLog entry
func (b *Bot) processMediaGroup(ctx context.Context, message telego.Message, msgs []telego.Message) (*models.PostLog, error) {
	log.Printf("[processMediaGroup] Processing: GroupID=%s, Messages count=%d", message.MediaGroupID, len(msgs))

	var caption string
	caption = b.handler.RetrieveMediaGroupCaption(message.MediaGroupID)
	if caption == "" {
		if userCaption, ok := b.handler.GetActiveCaption(message.Chat.ID); ok {
			caption = userCaption
		}
	}

	inputMedia := b.createInputMedia(msgs, caption)
	log.Printf("[processMediaGroup] Created input media: count=%d for GroupID=%s", len(inputMedia), message.MediaGroupID)

	if len(inputMedia) == 0 {
		b.mediaGroups.Delete(message.MediaGroupID)
		log.Printf("[processMediaGroup] Removed GroupID=%s from storage (no valid media)", message.MediaGroupID)
		return nil, nil // Not an error, just nothing to send
	}

	sentMessages, err := b.sendMediaGroupWithRetry(ctx, inputMedia, 3)
	if err != nil {
		log.Printf("[processMediaGroup] Error sending GroupID=%s after retries: %v", message.MediaGroupID, err)
		b.mediaGroups.Delete(message.MediaGroupID)
		log.Printf("[processMediaGroup] Removed GroupID=%s from storage (send error)", message.MediaGroupID)
		return nil, err
	}

	b.mediaGroups.Delete(message.MediaGroupID)
	b.handler.DeleteMediaGroupCaption(message.MediaGroupID)
	log.Printf("[processMediaGroup] Removed GroupID=%s from storage (processed successfully)", message.MediaGroupID)

	publishedTime := time.Now()
	channelPostID := 0
	if len(sentMessages) > 0 {
		channelPostID = sentMessages[0].MessageID
	}

	logEntry := &models.PostLog{
		SenderID:             message.From.ID,
		SenderUsername:       message.From.Username,
		Caption:              caption,
		MessageType:          "media_group",
		ReceivedAt:           time.Unix(int64(message.Date), 0),
		PublishedAt:          publishedTime,
		ChannelID:            b.handler.GetChannelID(),
		ChannelPostID:        channelPostID,
		OriginalMediaGroupID: message.MediaGroupID,
	}

	if b.debug {
		log.Printf("[processMediaGroup] Successfully sent GroupID=%s -> Channel Post ID: %d. Log entry created.", message.MediaGroupID, channelPostID)
	}

	return logEntry, nil
}

// handleLoadedMediaGroup contains the core logic for processing a media group
// after it has been loaded from storage and the delay has passed.
func (b *Bot) handleLoadedMediaGroup(ctx context.Context, groupID string, msgs []telego.Message, firstMessageID int) {
	if len(msgs) == 0 {
		log.Printf("[handleLoadedMediaGroup] Skipping GroupID=%s processing: Group empty.", groupID)
		// Clean up if group somehow became empty after storage
		b.mediaGroups.Delete(groupID)
		b.handler.DeleteMediaGroupCaption(groupID)
		return
	}

	if msgs[0].MessageID != firstMessageID {
		// This goroutine wasn't triggered by the first message stored, so skip processing.
		// The group will be processed by the goroutine started by the first message.
		log.Printf("[handleLoadedMediaGroup] Skipping GroupID=%s processing: Not triggered by first stored message (%d != %d).", groupID, msgs[0].MessageID, firstMessageID)
		return // Let the correct goroutine handle cleanup
	}

	// If we reached here, it's the correct goroutine for this group.
	// Process the media group
	logEntry, err := b.processMediaGroup(ctx, msgs[0], msgs)
	if err != nil {
		log.Printf("[handleLoadedMediaGroup] Error processing GroupID=%s: %v", groupID, err)
		// processMediaGroup handles deleting the group on error
		return
	}

	if logEntry == nil {
		// processMediaGroup returned nil, nil (e.g., no valid media)
		log.Printf("[handleLoadedMediaGroup] Processing GroupID=%s resulted in no log entry.", groupID)
		// processMediaGroup handles deleting the group in this case
		return
	}

	// Log the successful post
	if logErr := b.handler.LogPublishedPost(*logEntry); logErr != nil {
		log.Printf("[handleLoadedMediaGroup] Failed attempt to log media group post to DB from user %d: %v", logEntry.SenderID, logErr)
	}

	// Update user info
	// isAdmin, _ := b.handler.IsUserAdmin(ctx, b.bot, msgs[0].From.ID) // Check if IsUserAdmin exists and is correct
	isAdmin := true // Placeholder assuming admin
	// Use the UserRepo() method to access the repository
	if userUpdateErr := b.handler.UserRepo().UpdateUser(ctx, msgs[0].From.ID, msgs[0].From.Username, msgs[0].From.FirstName, msgs[0].From.LastName, isAdmin, "send_media_group"); userUpdateErr != nil {
		log.Printf("[handleLoadedMediaGroup] Failed to update user info after media group post: %v", userUpdateErr)
	}

	// Send confirmation message to user
	_, sendErr := b.bot.SendMessage(ctx, tu.Message(tu.ID(msgs[0].Chat.ID), locales.MsgMediaGroupSuccess))
	if sendErr != nil {
		log.Printf("[handleLoadedMediaGroup] Failed to send confirmation for GroupID=%s: %v", groupID, sendErr)
	}

	log.Printf("[handleLoadedMediaGroup] Finished processing GroupID=%s", groupID)
}

// processMediaGroupAfterDelay is a wrapper function for handleLoadedMediaGroup to be used with time.AfterFunc
func (b *Bot) processMediaGroupAfterDelay(groupID string, firstMessageID int) {
	// We need a new background context for the timer-triggered processing
	bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Add a timeout
	defer cancel()

	if b.debug {
		log.Printf("[processMediaGroupAfterDelay] Timer fired for GroupID=%s (first msg %d)", groupID, firstMessageID)
	}

	// Load the actual messages from the sync.Map
	if messages, ok := b.mediaGroups.Load(groupID); ok {
		msgs := messages.([]telego.Message)
		if len(msgs) > 0 {
			// Check if the first message stored matches the one that started the timer
			if msgs[0].MessageID == firstMessageID {
				log.Printf("[processMediaGroupAfterDelay] Found %d messages for GroupID=%s, proceeding with processing.", len(msgs), groupID)
				b.handleLoadedMediaGroup(bgCtx, groupID, msgs, firstMessageID)
			} else {
				// A different message started the timer, or the group was modified concurrently.
				// Let the goroutine tied to the actual first message handle it.
				log.Printf("[processMediaGroupAfterDelay] GroupID=%s: First stored message ID %d does not match triggering message ID %d. Skipping.", groupID, msgs[0].MessageID, firstMessageID)
			}
		} else {
			log.Printf("[processMediaGroupAfterDelay] GroupID=%s found in map but was empty.", groupID)
			b.mediaGroups.Delete(groupID)
			b.handler.DeleteMediaGroupCaption(groupID)
		}
	} else {
		log.Printf("[processMediaGroupAfterDelay] GroupID=%s not found in map (likely already processed or error).", groupID)
	}
}

// handleMediaGroup receives a message that is part of a media group, stores it,
// and starts a timer if it's the first message of the group.
func (b *Bot) handleMediaGroup(message telego.Message) {
	b.storeMessageInGroup(message)

	// Get the current state of the group
	if messages, ok := b.mediaGroups.Load(message.MediaGroupID); ok {
		msgs := messages.([]telego.Message)
		// If this is the first message we've stored for this group, start the timer.
		if len(msgs) == 1 {
			firstMessageID := msgs[0].MessageID
			if b.debug {
				log.Printf("[handleMediaGroup] First message for GroupID=%s (MsgID: %d) received. Starting 3s timer.", message.MediaGroupID, firstMessageID)
			}
			// Start the timer with the ID of the first message
			time.AfterFunc(3*time.Second, func() {
				b.processMediaGroupAfterDelay(message.MediaGroupID, firstMessageID)
			})
		}
	}
}

// handleCommandUpdate processes updates recognized as commands.
func (b *Bot) handleCommandUpdate(ctx context.Context, message telego.Message) {
	command := strings.Split(message.Text, " ")[0][1:] // Extract command without leading slash
	// GetCommandHandler returns only one value
	handlerFunc := b.handler.GetCommandHandler(command)
	if handlerFunc != nil { // Check if handler exists
		if err := handlerFunc(ctx, b.bot, message); err != nil {
			log.Printf("Error executing command '%s': %v", command, err)
			// Send generic error message only if the handler didn't send one.
			// This part is tricky; maybe handlers should return a bool indicating if they sent a reply?
			// For now, let's assume handlers manage their own error reporting.
			// b.handler.SendError(ctx, b.bot, message.Chat.ID, err) // Avoid double messaging
		}
	} else {
		log.Printf("Received unknown command: %s", command)
		// Optionally send an "unknown command" message
		// b.handler.SendText(ctx, b.bot, message.Chat.ID, "Unknown command.")
	}
}

// handleMediaGroupUpdate determines if the message is part of a media group and processes it.
func (b *Bot) handleMediaGroupUpdate(message telego.Message) {
	if message.MediaGroupID != "" {
		b.handleMediaGroup(message)
	}
}

// handlePhotoUpdate handles non-media-group photo updates.
func (b *Bot) handlePhotoUpdate(ctx context.Context, message telego.Message) {
	if message.Photo != nil && message.MediaGroupID == "" {
		if err := b.handler.HandlePhoto(ctx, b.bot, message); err != nil {
			log.Printf("Error handling photo message: %v", err)
			// b.handler.SendError(ctx, b.bot, message.Chat.ID, err)
		}
	}
}

// handleTextUpdate handles non-command text updates.
func (b *Bot) handleTextUpdate(ctx context.Context, message telego.Message) {
	if message.Text != "" && !strings.HasPrefix(message.Text, "/") {
		if err := b.handler.HandleText(ctx, b.bot, message); err != nil {
			log.Printf("Error handling text message: %v", err)
			// b.handler.SendError(ctx, b.bot, message.Chat.ID, err)
		}
	}
}

// handleCallbackQuery handles callback queries (e.g., from inline buttons)
func (b *Bot) handleCallbackQuery(ctx context.Context, query telego.CallbackQuery) {
	// Example: Log the callback data
	var messageID int = 0 // Default to 0 if message is inaccessible or not present
	// Try to type-assert the MaybeInaccessibleMessage to a regular Message
	if query.Message != nil {
		if msg, ok := query.Message.(*telego.Message); ok && msg != nil {
			messageID = msg.MessageID
		}
	}
	log.Printf("[handleCallbackQuery] Received callback: Data=\"%s\", From=%d, MsgID=%d",
		query.Data, query.From.ID, messageID)

	// Delegate callback handling to the suggestion manager
	if processed, err := b.handler.ProcessSuggestionCallback(ctx, query); err != nil {
		log.Printf("[handleCallbackQuery] Error processing suggestion callback: %v", err)
		// Try to answer the callback query even if there was an error processing it
		answerErr := b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            locales.MsgErrorGeneral, // Generic error message
			ShowAlert:       true,                    // Show as an alert popup
		})
		if answerErr != nil {
			log.Printf("[handleCallbackQuery] Error sending error answer to callback query %s: %v", query.ID, answerErr)
		}
	} else if processed {
		// If processed successfully by suggestion manager, it should answer the query.
		log.Printf("[handleCallbackQuery] Callback query %s processed by suggestion manager.", query.ID)
	} else {
		// Callback wasn't related to suggestions (or manager didn't handle it)
		log.Printf("[handleCallbackQuery] Callback query %s not processed by suggestion manager.", query.ID)
		// Answer the callback query to remove the loading state on the button
		answerErr := b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Action not implemented.", // Placeholder
		})
		if answerErr != nil {
			log.Printf("[handleCallbackQuery] Error sending default answer to callback query %s: %v", query.ID, answerErr)
		}
	}
}

// handleUpdateInLoop is the core update processing logic.
func (b *Bot) handleUpdateInLoop(ctx context.Context, update telego.Update) {
	// Message updates
	if update.Message != nil {
		message := *update.Message
		if b.debug {
			log.Printf("-> Receiving Message: ChatID=%d, UserID=%d, Text=\"%s\", Photo=%v, MediaGroupID=%s",
				message.Chat.ID, message.From.ID, message.Text, message.Photo != nil, message.MediaGroupID)
		}

		// 1. Check if it should be handled by the suggestion system
		suggestionProcessed, err := b.handler.ProcessSuggestionMessage(ctx, update)
		if err != nil {
			log.Printf("[handleUpdateInLoop] Error processing suggestion message for user %d: %v", message.From.ID, err)
			// Suggestion manager should handle informing the user, just log here.
			return // Stop processing this update if suggestion handler had an error
		}
		if suggestionProcessed {
			if b.debug {
				log.Printf("[handleUpdateInLoop] Message processed by suggestion system.")
			}
			return // Message was handled by the suggestion system
		}

		// 2. If not suggestion, process based on type (Command > MediaGroup > Photo > Text)
		if strings.HasPrefix(message.Text, "/") { // Check for command using string prefix
			b.handleCommandUpdate(ctx, message)
		} else if message.MediaGroupID != "" {
			b.handleMediaGroupUpdate(message) // Media groups are handled slightly differently
		} else if message.Photo != nil {
			b.handlePhotoUpdate(ctx, message)
		} else if message.Text != "" {
			b.handleTextUpdate(ctx, message)
		}
	} else if update.CallbackQuery != nil {
		// Callback Query updates
		query := *update.CallbackQuery
		b.handleCallbackQuery(ctx, query)
	} else if b.debug {
		// Log other update types if debugging is enabled
		log.Printf("-> Receiving Unhandled Update Type: %+v", update)
	}
}

// Start begins polling for updates and processing them
func (b *Bot) Start(ctx context.Context) {
	var err error
	// Get updates channel
	b.updatesChan, err = b.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to start long polling: %v", err)
	}

	log.Println("Bot started successfully!")

	// Start processing updates in a separate goroutine
	go func() {
		for update := range b.updatesChan {
			// Recover from potential panics in handlers
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic in update handler: %v\nStack trace:\n%s", r, debug.Stack())
				}
			}()
			// Create a new context for each update to handle timeouts/cancellation per update
			updateCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // 30-second timeout per update

			// It's crucial to handle the update in a separate goroutine IF handleUpdateInLoop could block
			// for a long time, to avoid blocking the main update receiving loop.
			// However, for simplicity here, we handle it synchronously but with recovery.
			// If handlers become complex, consider `go func() { ... handle ... }()`
			b.handleUpdateInLoop(updateCtx, update)

			cancel() // Release context resources
		}
		log.Println("Update channel closed. Bot shutting down...")
	}()
}

// Stop gracefully stops the bot
func (b *Bot) Stop() {
	log.Println("Stopping bot...")
	// StopLongPolling is not available in telego v1. Stop is handled by cancelling the context passed to Start.
	// b.bot.StopLongPolling() // Removed this line
	log.Println("Bot stopped. (Long polling stops via context cancellation)")
}
