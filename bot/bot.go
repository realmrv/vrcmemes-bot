package bot

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	dbi "vrcmemes-bot/internal/database" // Corrected import path
	"vrcmemes-bot/internal/locales"

	"github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Bot represents the main application logic for the Telegram bot.
// It wraps the telego library, manages the update loop, handles different update types,
// and orchestrates complex operations like media group processing.
type Bot struct {
	bot           *telego.Bot           // The underlying telego bot instance.
	updatesChan   <-chan telego.Update  // Channel for receiving updates from Telegram.
	mediaGroups   sync.Map              // Thread-safe map for media groups.
	debug         bool                  // Enable debug logging.
	channelID     int64                 // Channel ID for posting.
	captionProv   dbi.CaptionProvider   // Provides captions.
	postLogger    dbi.PostLogger        // Logs published posts.
	handerProv    dbi.HandlerProvider   // Provides message handlers (commands, text, photo).
	suggestionMgr dbi.SuggestionManager // Handles suggestion flow.
	callbackProc  dbi.CallbackProcessor // Processes callback queries.
	userRepo      dbi.UserRepository    // Repository for user data.
	actionLogger  dbi.UserActionLogger  // Logs user actions.
}

// BotDeps holds the dependencies required by the Bot.
type BotDeps struct {
	Bot           *telego.Bot
	Debug         bool
	ChannelID     int64
	CaptionProv   dbi.CaptionProvider
	PostLogger    dbi.PostLogger
	HandlerProv   dbi.HandlerProvider
	SuggestionMgr dbi.SuggestionManager
	CallbackProc  dbi.CallbackProcessor
	UserRepo      dbi.UserRepository
	ActionLogger  dbi.UserActionLogger
}

// New creates a new Bot instance from its dependencies.
// Returns the new Bot instance or an error if dependencies are missing.
func New(deps BotDeps) (*Bot, error) {
	// Validate dependencies
	if deps.Bot == nil {
		return nil, fmt.Errorf("telego bot instance cannot be nil")
	}
	if deps.CaptionProv == nil {
		return nil, fmt.Errorf("caption provider cannot be nil")
	}
	if deps.PostLogger == nil {
		return nil, fmt.Errorf("post logger cannot be nil")
	}
	if deps.HandlerProv == nil {
		return nil, fmt.Errorf("handler provider cannot be nil")
	}
	if deps.SuggestionMgr == nil {
		return nil, fmt.Errorf("suggestion manager cannot be nil")
	}
	if deps.CallbackProc == nil {
		return nil, fmt.Errorf("callback processor cannot be nil")
	}
	if deps.UserRepo == nil {
		return nil, fmt.Errorf("user repository cannot be nil")
	}
	if deps.ActionLogger == nil {
		return nil, fmt.Errorf("action logger cannot be nil")
	}

	return &Bot{
		bot:           deps.Bot,
		debug:         deps.Debug,
		channelID:     deps.ChannelID,
		captionProv:   deps.CaptionProv,
		postLogger:    deps.PostLogger,
		handerProv:    deps.HandlerProv,
		suggestionMgr: deps.SuggestionMgr,
		callbackProc:  deps.CallbackProc,
		userRepo:      deps.UserRepo,
		actionLogger:  deps.ActionLogger,
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

	handlerFunc := b.handerProv.GetCommandHandler(command) // Use handler provider
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
		err := b.handerProv.HandlePhoto(ctx, b.bot, message) // Use handler provider
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

		// Proceed with the general text handler
		err := b.handerProv.HandleText(ctx, b.bot, message) // Use handler provider
		if err != nil {
			log.Printf("%s General text handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s general text handler error: %w", logPrefix, err))
		}
	} else {
		log.Printf("%s Ignoring empty or command message in text handler", logPrefix)
	}
}

// handleVideoUpdate processes an incoming single video message.
// Placeholder: Implement actual video handling logic if needed.
func (b *Bot) handleVideoUpdate(ctx context.Context, message telego.Message) {
	logPrefix := fmt.Sprintf("[Video User:%d Msg:%d]", message.From.ID, message.MessageID)
	if message.Video != nil && message.MediaGroupID == "" {
		if b.debug {
			log.Printf("%s Processing single video", logPrefix)
		}
		// TODO: Call a dedicated handler for single videos if needed
		// err := b.handler.HandleVideo(ctx, b.bot, message)
		// if err != nil {
		// 	log.Printf("%s Handler error: %v", logPrefix, err)
		// 	sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		// }
		log.Printf("%s Placeholder for single video handling.", logPrefix)
	} else {
		log.Printf("%s Ignoring non-single-video message in video handler", logPrefix)
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
	processed, err := b.suggestionMgr.ProcessSuggestionCallback(ctx, query) // Use suggestion manager
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

	// If not handled by suggestion manager, delegate to general callback processor
	// This assumes ProcessSuggestionCallback is also part of the CallbackProcessor interface
	processedGeneral, errGeneral := b.callbackProc.ProcessSuggestionCallback(ctx, query) // Use callback processor
	if errGeneral != nil {
		log.Printf("%s General callback handler error: %v", logPrefix, errGeneral)
		sentry.CaptureException(fmt.Errorf("%s general callback handler error: %w", logPrefix, errGeneral))
		// Answer with a localized generic error
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: errorMsg})
		return
	}
	if processedGeneral {
		if b.debug {
			log.Printf("%s Callback handled by general processor", logPrefix)
		}
		return
	}

	// If not handled by suggestion manager or general processor
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

		// --- Suggestion Manager Handling FIRST ---
		if b.suggestionMgr != nil { // Check if suggestion manager exists
			processedBySuggestionManager, suggestionErr := b.suggestionMgr.HandleMessage(ctx, update)
			if suggestionErr != nil {
				// Log error and potentially notify user, but stop processing here
				logPrefix := fmt.Sprintf("[User:%d]", message.From.ID)
				log.Printf("%s Suggestion handler error: %v", logPrefix, suggestionErr)
				sentry.CaptureException(fmt.Errorf("%s suggestion handler error: %w", logPrefix, suggestionErr))
				return // Stop further processing on suggestion handler error
			}
			// Log the result of the suggestion manager handling
			log.Printf("[Bot UpdateLoop User:%d] Suggestion manager HandleMessage result: processed=%v", message.From.ID, processedBySuggestionManager)

			if processedBySuggestionManager {
				if b.debug {
					log.Printf("[Bot UpdateLoop User:%d] Message handled by suggestion manager. Skipping standard handlers.", message.From.ID)
				}
				return // IMPORTANT: Stop further processing if manager handled it
			}
			// If suggestion manager didn't process, fall through to standard handlers
		}
		// --- End Suggestion Manager Handling ---

		// Proceed with standard handlers ONLY if not handled by suggestion manager
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
		case message.Video != nil:
			b.handleVideoUpdate(ctx, message) // Handle single videos
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
