package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/handlers"

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

// New creates a new bot instance, accepting a pre-configured MessageHandler
func New(token string, debug bool, handler *handlers.MessageHandler) (*Bot, error) {
	var bot *telego.Bot
	var err error

	if debug {
		bot, err = telego.NewBot(token, telego.WithDefaultDebugLogger())
	} else {
		bot, err = telego.NewBot(token, telego.WithDefaultLogger(false, false))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create telego bot: %w", err)
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
		log.Printf("[handleLoadedMediaGroup] Error logging processed GroupID=%s: %v", groupID, logErr)
	}
	// processMediaGroup handles deleting the group on success
}

// processMediaGroupAfterDelay handles the logic for processing a media group after a delay.
// It's intended to be run in a goroutine.
func (b *Bot) processMediaGroupAfterDelay(groupID string, firstMessageID int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		log.Printf("[processMediaGroupAfterDelay] Context cancelled before processing GroupID=%s: %v", groupID, ctx.Err())
		b.mediaGroups.Delete(groupID) // Clean up map entry if processing is aborted
		b.handler.DeleteMediaGroupCaption(groupID)
		return
	}

	if messages, exists := b.mediaGroups.Load(groupID); exists {
		if msgs, ok := messages.([]telego.Message); ok {
			// Delegate actual processing to the new function
			b.handleLoadedMediaGroup(ctx, groupID, msgs, firstMessageID)
		} else {
			// Should not happen if storeMessageInGroup works correctly
			log.Printf("[processMediaGroupAfterDelay] Error: Value for GroupID %s is not []telego.Message", groupID)
			b.mediaGroups.Delete(groupID)
			b.handler.DeleteMediaGroupCaption(groupID)
		}
	} else {
		// Group might have been processed already and deleted by another goroutine, or timed out.
		log.Printf("[processMediaGroupAfterDelay] GroupID=%s not found in storage after delay (likely already processed or timed out).", groupID)
	}
}

// handleMediaGroup starts processing for a media group message
func (b *Bot) handleMediaGroup(message telego.Message) {
	log.Printf("[handleMediaGroup] Received: ID=%d, GroupID=%s", message.MessageID, message.MediaGroupID)
	b.storeMessageInGroup(message)

	groupID := message.MediaGroupID
	firstMessageID := message.MessageID

	go func() {
		b.processMediaGroupAfterDelay(groupID, firstMessageID)
	}()
}

// handleCommandUpdate processes a command message.
func (b *Bot) handleCommandUpdate(ctx context.Context, message telego.Message) {
	parts := strings.SplitN(message.Text, " ", 2)
	command := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	if i := strings.Index(command, "@"); i != -1 {
		command = command[:i]
	}

	log.Printf("[handleCommandUpdate] Received command: /%s from UserID: %d", command, message.From.ID)
	cmdHandlerFunc := b.handler.GetCommandHandler(command)
	if cmdHandlerFunc != nil {
		if err := cmdHandlerFunc(ctx, b.bot, message); err != nil {
			log.Printf("[handleCommandUpdate] Error executing handler for /%s: %v", command, err)
			_, _ = b.bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), "An error occurred processing the command."))
		}
	} else {
		log.Printf("[handleCommandUpdate] Unknown command: /%s from UserID: %d", command, message.From.ID)
	}
}

// handleMediaGroupUpdate handles the initial trigger for a media group.
func (b *Bot) handleMediaGroupUpdate(message telego.Message) {
	b.handleMediaGroup(message) // Delegates to the existing function that starts the goroutine
}

// handlePhotoUpdate processes a single photo message.
func (b *Bot) handlePhotoUpdate(ctx context.Context, message telego.Message) {
	if err := b.handler.HandlePhoto(ctx, b.bot, message); err != nil {
		log.Printf("[handlePhotoUpdate] Error handling photo: %v", err)
	}
}

// handleTextUpdate processes a regular text message.
func (b *Bot) handleTextUpdate(ctx context.Context, message telego.Message) {
	if err := b.handler.HandleText(ctx, b.bot, message); err != nil {
		log.Printf("[handleTextUpdate] Error handling text: %v", err)
	}
}

// handleUpdateInLoop processes a single update inside the manual loop
func (b *Bot) handleUpdateInLoop(ctx context.Context, update telego.Update) {
	if update.Message == nil {
		return // Ignore non-message updates for now
	}
	message := *update.Message

	// --- Route based on message content ---

	if strings.HasPrefix(message.Text, "/") {
		// Handle Command
		b.handleCommandUpdate(ctx, message)
	} else if message.MediaGroupID != "" {
		// Handle Media Group
		b.handleMediaGroupUpdate(message) // Starts goroutine
	} else if message.Photo != nil {
		// Handle Single Photo
		b.handlePhotoUpdate(ctx, message)
	} else if message.Text != "" {
		// Handle Text Message
		b.handleTextUpdate(ctx, message)
	} else {
		// --- Log unhandled message types ---
		if b.debug {
			msgTypeUnhandled := "unknown"
			if message.Video != nil {
				msgTypeUnhandled = "video"
			} else if message.Document != nil {
				msgTypeUnhandled = "document"
			} else if message.Audio != nil {
				msgTypeUnhandled = "audio"
			} // ... add other types ...
			log.Printf("[handleUpdateInLoop] Received unhandled message type '%s' from UserID: %d", msgTypeUnhandled, message.From.ID)
		}
	}
}

// Start starts the bot and processes updates manually
func (b *Bot) Start(ctx context.Context) {
	log.Println("Starting bot with manual update processing loop...")

	updates, err := b.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout:        60,
		AllowedUpdates: []string{"message"},
	})
	if err != nil {
		// If context is cancelled during initial poll, it might be a graceful shutdown
		if ctx.Err() != nil {
			log.Printf("Initial update polling stopped due to context cancellation: %v", ctx.Err())
			return
		}
		log.Fatalf("Failed to get updates via long polling: %v", err)
	}
	b.updatesChan = updates // Store the channel (though loop below uses it directly)

	// Manual update processing loop
	for {
		select {
		case <-ctx.Done(): // Handle graceful shutdown
			log.Println("Context cancelled, stopping manual update loop...")
			// Perform any necessary cleanup before returning
			return

		case update, ok := <-updates: // Process updates from channel
			if !ok {
				log.Println("Updates channel closed, stopping loop.")
				// This might happen if the context used in UpdatesViaLongPolling is cancelled internally
				// Or if there's an unrecoverable error in polling.
				return
			}
			// Process the received update
			// Use a background context for the handler logic itself for now
			b.handleUpdateInLoop(context.Background(), update)
		}
	}
}

// Stop is called during graceful shutdown (initiated by context cancellation)
func (b *Bot) Stop() {
	log.Println("Stop method called (manual loop handles shutdown via context)")
	// Add any Bot-specific cleanup here if needed (e.g., waiting for goroutines)
}
