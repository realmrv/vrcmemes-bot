package bot

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"

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
		// Send localized "unknown command" message
		lang := locales.DefaultLanguage // Default to Russian
		if message.From != nil && message.From.LanguageCode != "" {
			// lang = message.From.LanguageCode
		}
		localizer := locales.NewLocalizer(lang)
		unknownCmdMsg := locales.GetMessage(localizer, "MsgErrorUnknownCommand", nil, nil)

		_, err := b.bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), unknownCmdMsg))
		if err != nil {
			log.Printf("%s Failed to send unknown command message: %v", logPrefix, err)
		}
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

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if query.From.LanguageCode != "" {
		// lang = query.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Delegate to suggestion manager first
	processed, err := b.handler.ProcessSuggestionCallback(ctx, query)
	if err != nil {
		log.Printf("%s Suggestion callback handler error: %v", logPrefix, err)
		sentry.CaptureException(fmt.Errorf("%s suggestion callback handler error: %w", logPrefix, err))
		// Answer with a localized generic error
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: errorMsg})
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
	// Answer the callback query with localized message
	notImplementedMsg := locales.GetMessage(localizer, "MsgErrorNotImplemented", nil, nil)
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
			sentry.CaptureException(fmt.Errorf("%s", errMsg))
			// Optionally send a message to the user/admin if possible?
		}
	}()

	// Determine update type and call appropriate handler
	switch {
	case update.Message != nil:
		message := *update.Message
		userID := message.From.ID

		// --- Suggestion Manager Handling ---
		// Check if the suggestion manager should handle this message first
		// This includes states like awaiting content for /suggest
		suggestionMgr := b.handler.SuggestionManager() // Get manager via getter
		if suggestionMgr != nil {                      // Check if the returned manager instance is nil
			processedBySuggestionManager, suggestionErr := suggestionMgr.HandleMessage(ctx, update)
			if suggestionErr != nil {
				logPrefix := fmt.Sprintf("[User:%d]", userID)
				log.Printf("%s Suggestion handler error: %v", logPrefix, suggestionErr)
				sentry.CaptureException(fmt.Errorf("%s suggestion handler error: %w", logPrefix, suggestionErr))
				// Suggestion manager should ideally handle user feedback directly
				return // Stop further processing if manager handled it (even with error)
			}
			if processedBySuggestionManager {
				if b.debug {
					log.Printf("[User:%d] Message handled by suggestion manager.", userID)
				}
				return // Stop further processing if manager handled it successfully
			}
		}
		// --- End Suggestion Manager Handling ---

		// If not handled by suggestion manager, proceed with standard handlers
		switch {
		case message.Text != "" && strings.HasPrefix(message.Text, "/"):
			b.handleCommandUpdate(ctx, message)
		case message.MediaGroupID != "":
			// Call the handler from mediagroup.go
			b.handleMediaGroupUpdate(message)
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
	// Get updates channel. Pass the main context to UpdatesViaLongPolling.
	// The library should respect context cancellation for graceful shutdown.
	// Explicitly allow message and callback_query updates.
	updatesParams := &telego.GetUpdatesParams{
		AllowedUpdates: []string{"message", "callback_query"},
		// You can adjust Timeout if needed, default is 0 (which implies long polling default)
		// Timeout: 60, // Example: 60 seconds timeout
	}
	b.updatesChan, err = b.bot.UpdatesViaLongPolling(ctx, updatesParams)
	if err != nil {
		log.Fatalf("Failed to get updates channel: %v", err)
	}

	log.Println("Bot started successfully. Polling for updates...")

	// Start processing updates in a separate goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Update processing loop stopped due to context cancellation.")
				return
			case update, ok := <-b.updatesChan:
				if !ok {
					log.Println("Updates channel closed gracefully.")
					return
				}
				// Process each update in its own goroutine to avoid blocking the loop
				// Use a copy of the update variable for the goroutine closure
				updateCopy := update
				// Pass the main context down to the handler
				go b.handleUpdateInLoop(ctx, updateCopy)
			}
		}
	}()
}

// Stop gracefully shuts down the bot.
// For telego v1, stopping is primarily handled by cancelling the context passed to Start.
// This method is kept for potential future cleanup logic or compatibility.
func (b *Bot) Stop() {
	log.Println("Stop method called. Shutdown is triggered by cancelling the main context.")
	// No explicit StopLongPolling call needed for telego v1 with context cancellation.
	// If there were other resources to clean up (e.g., closing connections not handled by defer),
	// they would go here.
}
