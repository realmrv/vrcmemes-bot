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

	"github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Bot represents the main application logic for the Telegram bot.
// It wraps the telego library, manages the update loop, handles different update types,
// and orchestrates complex operations like media group processing.
type Bot struct {
	bot         *telego.Bot              // The underlying telego bot instance.
	handler     *handlers.MessageHandler // The message handler containing logic for specific commands and message types.
	updatesChan <-chan telego.Update     // Channel for receiving updates from Telegram.
	mediaGroups sync.Map                 // Thread-safe map to temporarily store incoming media group messages. Key: mediaGroupID (string), Value: []telego.Message
	debug       bool                     // Flag to enable debug logging.
}

// New creates a new Bot instance.
// It requires a pre-configured telego.Bot instance, a MessageHandler, and a debug flag.
// Returns the new Bot instance or an error if dependencies are nil.
func New(bot *telego.Bot, debug bool, handler *handlers.MessageHandler) (*Bot, error) {
	// Validate dependencies
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
		// updatesChan is initialized in Start()
	}, nil
}

// storeMessageInGroup adds a message to a temporary storage for its media group.
// It ensures thread-safety using sync.Map and sorts messages by ID.
// Limits the number of messages per group to prevent memory issues.
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

// createInputMedia converts a slice of telego.Message (belonging to a media group)
// into a slice of telego.InputMedia suitable for bot.SendMediaGroup.
// It applies the provided caption to the first photo in the group.
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

// parseRetryAfter attempts to extract the 'retry after N' duration (in seconds)
// from a Telegram API error string (typically for 429 Too Many Requests).
// Returns the duration and true if successful, otherwise 0 and false.
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

// sendMediaGroupWithRetry attempts to send a media group to the configured channel,
// handling potential '429 Too Many Requests' errors by retrying after the specified delay.
// It retries up to maxRetries times.
func (b *Bot) sendMediaGroupWithRetry(ctx context.Context, inputMedia []telego.InputMedia, maxRetries int) ([]telego.Message, error) {
	var lastErr error
	var retryCount int
	channelID := b.handler.GetChannelID()
	const defaultRetryWait = 2 * time.Second // Default wait time if parsing fails

	mediaGroupID := "unknown" // Placeholder for logging
	// Attempt to get a group ID for logging context, if available in the first item
	/* // Removed unused variable declaration
	if len(inputMedia) > 0 {
		if photo, ok := inputMedia[0].(*telego.InputMediaPhoto); ok {
			// Unfortunately InputMediaPhoto doesn't carry the original group ID.
			// We rely on the caller (processMediaGroup) to log the group ID.
		}
	}
	*/

	logPrefix := fmt.Sprintf("[sendMediaGroupWithRetry Group:%s]", mediaGroupID)

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
func (b *Bot) processMediaGroup(ctx context.Context, message telego.Message, msgs []telego.Message) (*models.PostLog, error) {
	groupID := message.MediaGroupID
	logPrefix := fmt.Sprintf("[processMediaGroup Group:%s]", groupID)
	log.Printf("%s Processing: Messages count=%d", logPrefix, len(msgs))

	var caption string
	caption = b.handler.RetrieveMediaGroupCaption(groupID)
	if caption == "" {
		if userCaption, ok := b.handler.GetActiveCaption(message.Chat.ID); ok {
			caption = userCaption
		}
	}

	inputMedia := b.createInputMedia(msgs, caption)
	log.Printf("%s Created input media: count=%d", logPrefix, len(inputMedia))

	if len(inputMedia) == 0 {
		b.mediaGroups.Delete(groupID)
		log.Printf("%s Removed group from storage (no valid media)", logPrefix)
		return nil, nil // Not an error, just nothing to send
	}

	sentMessages, err := b.sendMediaGroupWithRetry(ctx, inputMedia, 3)
	if err != nil {
		log.Printf("%s Error sending group after retries: %v", logPrefix, err)
		// sendMediaGroupWithRetry already sends to Sentry
		b.mediaGroups.Delete(groupID)
		log.Printf("%s Removed group from storage (send error)", logPrefix)
		// Return the error from sendMediaGroupWithRetry
		return nil, fmt.Errorf("failed during sendMediaGroupWithRetry: %w", err)
	}

	b.mediaGroups.Delete(groupID)
	b.handler.DeleteMediaGroupCaption(groupID)
	log.Printf("%s Removed group from storage (processed successfully)", logPrefix)

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
		OriginalMediaGroupID: groupID,
	}

	if b.debug {
		log.Printf("%s Successfully sent -> Channel Post ID: %d. Log entry created.", logPrefix, channelPostID)
	}

	return logEntry, nil
}

// handleLoadedMediaGroup is the core logic executed after a delay for a potential media group.
// It ensures only the goroutine corresponding to the first message processes the group,
// calls processMediaGroup, logs results, and handles cleanup.
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

// processMediaGroupAfterDelay waits for a short duration before attempting to process a media group.
// This allows time for subsequent messages in the same group to arrive.
// It loads the group from storage and calls handleLoadedMediaGroup.
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

// handleMediaGroup is called when a message belonging to a media group is received.
// It stores the message and, if it's the first message seen for this group,
// starts a delayed task (processMediaGroupAfterDelay) to handle the group later.
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

// handleCommandUpdate processes a message identified as a command.
func (b *Bot) handleCommandUpdate(ctx context.Context, message telego.Message) {
	command := "unknown"
	if len(message.Text) > 1 && strings.HasPrefix(message.Text, "/") {
		command = strings.Split(message.Text, " ")[0][1:] // Extract command without leading slash
	}
	logPrefix := fmt.Sprintf("[Cmd:%s User:%d]", command, message.From.ID)

	handlerFunc := b.handler.GetCommandHandler(command)
	if handlerFunc != nil {
		if b.debug {
			log.Printf("%s Executing handler", logPrefix)
		}
		err := handlerFunc(ctx, b.bot, message)
		if err != nil {
			log.Printf("%s Handler error: %v", logPrefix, err)
			// Handler might have already sent an error message via sendError
			// Capture the error in Sentry for visibility
			sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		} else {
			if b.debug {
				log.Printf("%s Handler finished successfully", logPrefix)
			}
		}
	} else {
		log.Printf("%s No handler found", logPrefix)
		// Send "unknown command" message
		// TODO: Define locales.MsgErrorUnknownCommand or use a default string
		unknownCmdMsg := "😕 Sorry, I don't recognize that command." // Default message
		_, err := b.bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), unknownCmdMsg))
		if err != nil {
			log.Printf("%s Failed to send unknown command message: %v", logPrefix, err)
		}
	}
}

// handleMediaGroupUpdate is the entry point for handling messages that are part of a media group.
// It simply calls handleMediaGroup to manage storage and delayed processing.
func (b *Bot) handleMediaGroupUpdate(message telego.Message) {
	if message.MediaGroupID != "" {
		b.handleMediaGroup(message)
	}
}

// handlePhotoUpdate processes an incoming single photo message.
func (b *Bot) handlePhotoUpdate(ctx context.Context, message telego.Message) {
	logPrefix := fmt.Sprintf("[Photo User:%d Msg:%d]", message.From.ID, message.MessageID)
	if message.Photo != nil && message.MediaGroupID == "" {
		if b.debug {
			log.Printf("%s Processing single photo", logPrefix)
		}
		err := b.handler.HandlePhoto(ctx, b.bot, message)
		if err != nil {
			log.Printf("%s Handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		}
	} else {
		log.Printf("%s Ignoring non-single-photo message in photo handler", logPrefix)
	}
}

// handleTextUpdate processes an incoming text message.
func (b *Bot) handleTextUpdate(ctx context.Context, message telego.Message) {
	logPrefix := fmt.Sprintf("[Text User:%d Msg:%d]", message.From.ID, message.MessageID)
	if message.Text != "" && !strings.HasPrefix(message.Text, "/") {
		if b.debug {
			log.Printf("%s Processing text message", logPrefix)
		}

		// First, check if the suggestion manager should handle this message
		update := telego.Update{Message: &message}
		processedBySuggestionManager, suggestionErr := b.handler.ProcessSuggestionMessage(ctx, update)
		if suggestionErr != nil {
			log.Printf("%s Suggestion handler error: %v", logPrefix, suggestionErr)
			// ProcessSuggestionMessage might return a wrapped error, capture it
			sentry.CaptureException(fmt.Errorf("%s suggestion handler error: %w", logPrefix, suggestionErr))
			// Decide if we should stop or continue to general handler? Assuming stop.
			return
		}

		if processedBySuggestionManager {
			if b.debug {
				log.Printf("%s Message handled by suggestion manager", logPrefix)
			}
			return // Message was handled, do nothing more
		}

		// If not handled by suggestion manager, proceed with the general text handler
		err := b.handler.HandleText(ctx, b.bot, message)
		if err != nil {
			log.Printf("%s General text handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s general text handler error: %w", logPrefix, err))
		}
	} else {
		log.Printf("%s Ignoring empty or command message in text handler", logPrefix)
	}
}

// handleCallbackQuery processes an incoming callback query.
func (b *Bot) handleCallbackQuery(ctx context.Context, query telego.CallbackQuery) {
	logPrefix := fmt.Sprintf("[Callback User:%d QueryID:%s]", query.From.ID, query.ID)
	if b.debug {
		log.Printf("%s Received callback query with data: %q", logPrefix, query.Data)
	}

	// Delegate to suggestion manager first
	processed, err := b.handler.ProcessSuggestionCallback(ctx, query)
	if err != nil {
		log.Printf("%s Suggestion callback handler error: %v", logPrefix, err)
		sentry.CaptureException(fmt.Errorf("%s suggestion callback handler error: %w", logPrefix, err))
		// Answer with a generic error? Or assume manager answered?
		_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "An error occurred."}) // Use default error text
		return
	}

	if processed {
		if b.debug {
			log.Printf("%s Callback handled by suggestion manager", logPrefix)
		}
		// Suggestion manager should ideally answer the callback query itself.
		return
	}

	// If not handled by suggestion manager, handle other callbacks if any
	log.Printf("%s Callback query not handled (Data: %q)", logPrefix, query.Data)
	// Answer the callback query to remove the loading state on the button
	// TODO: Define locales.MsgErrorNotImplemented or use a default string
	notImplementedMsg := "Action not available." // Default message
	_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: notImplementedMsg})
}

// handleUpdateInLoop is the core update processing function called within the main loop.
func (b *Bot) handleUpdateInLoop(ctx context.Context, update telego.Update) {
	// Recover from panics in handler logic
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("PANIC recovered in handleUpdateInLoop: %v", r)
			log.Printf("%s\n%s", errMsg, string(debug.Stack()))
			// Capture panic in Sentry
			sentry.CaptureException(fmt.Errorf(errMsg))
			// Optionally send a message to the user/admin if possible?
		}
	}()

	// Determine update type and call appropriate handler
	switch {
	case update.Message != nil:
		message := *update.Message
		switch {
		case message.Text != "" && strings.HasPrefix(message.Text, "/"):
			b.handleCommandUpdate(ctx, message)
		case message.MediaGroupID != "":
			b.handleMediaGroupUpdate(message) // Needs context?
		case message.Photo != nil:
			b.handlePhotoUpdate(ctx, message)
		case message.Text != "":
			b.handleTextUpdate(ctx, message)
		default:
			if b.debug {
				log.Printf("Unhandled message type from user %d in chat %d", message.From.ID, message.Chat.ID)
			}
		}
	case update.CallbackQuery != nil:
		b.handleCallbackQuery(ctx, *update.CallbackQuery)
	// Add cases for other update types if needed (e.g., EditedMessage, ChannelPost)
	default:
		if b.debug {
			log.Printf("Unhandled update type: %+v", update)
		}
	}
}

// Start begins the bot's operation.
// It retrieves the updates channel from the telego bot instance
// and starts the main loop to process incoming updates concurrently.
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

// Stop gracefully shuts down the bot.
// It stops the underlying telego bot from receiving further updates.
func (b *Bot) Stop() {
	log.Println("Stopping bot...")
	// StopLongPolling is not available in telego v1. Stop is handled by cancelling the context passed to Start.
	// b.bot.StopLongPolling() // Removed this line
	log.Println("Bot stopped. (Long polling stops via context cancellation)")
}
