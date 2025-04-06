package bot

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	dbi "vrcmemes-bot/internal/database" // Corrected import path
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/internal/mediagroups"

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
	mediaGroupMgr *mediagroups.Manager  // Handles generic media group processing.
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
	MediaGroupMgr *mediagroups.Manager // Add manager dependency
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
	if deps.MediaGroupMgr == nil {
		return nil, fmt.Errorf("media group manager cannot be nil")
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
		mediaGroupMgr: deps.MediaGroupMgr,
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
func (b *Bot) handleVideoUpdate(ctx context.Context, message telego.Message) {
	logPrefix := fmt.Sprintf("[Video User:%d Msg:%d]", message.From.ID, message.MessageID)
	if message.Video != nil && message.MediaGroupID == "" {
		if b.debug {
			log.Printf("%s Processing single video", logPrefix)
		}
		// Call the dedicated handler for single videos
		err := b.handerProv.HandleVideo(ctx, b.bot, message) // Use handler provider
		if err != nil {
			log.Printf("%s Handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		}
		// log.Printf("%s Placeholder for single video handling.", logPrefix)
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
			// Delegate to the universal media group manager
			// Pass the context, message, and the specific processor function for admin posts
			err := b.mediaGroupMgr.HandleMessage(
				ctx, // Pass the context from the update loop
				message,
				b.processAdminMediaGroup,        // The function to call when the group is ready
				mediagroups.DefaultProcessDelay, // Use default delay
				mediagroups.DefaultMaxGroupSize, // Use default max size
			)
			if err != nil {
				log.Printf("[Bot UpdateLoop User:%d] Error handling media group via manager: %v", message.From.ID, err)
				// Consider sending error to user/Sentry
			}
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

// --- Media Group Processing Logic (Moved from mediagroup.go) ---

const (
	adminMediaGroupMaxSendRetries = 3 // Retries specific to admin posts
)

// processAdminMediaGroup is the specific handler function for media groups sent directly by admins.
// This function matches the mediagroups.ProcessFunc signature.
func (b *Bot) processAdminMediaGroup(ctx context.Context, groupID string, msgs []telego.Message) error {
	if len(msgs) == 0 {
		log.Printf("[ProcessAdminMediaGroup Group:%s] Attempted to process empty group.", groupID)
		return nil // Nothing to process
	}
	firstMessage := msgs[0] // Use the first message for context like sender ID
	logPrefix := fmt.Sprintf("[ProcessAdminMediaGroup Group:%s User:%d]", groupID, firstMessage.From.ID)
	log.Printf("%s Processing %d messages.", logPrefix, len(msgs))

	// Retrieve caption (check group-specific first, then user's active caption)
	caption := b.captionProv.RetrieveMediaGroupCaption(groupID)
	if caption == "" {
		if userCaption, ok := b.captionProv.GetActiveCaption(firstMessage.Chat.ID); ok {
			caption = userCaption
			log.Printf("%s Using active user caption for group.", logPrefix)
		}
	}

	inputMedia := createInputMedia(msgs, caption) // Use helper from helpers.go
	log.Printf("%s Created %d input media items.", logPrefix, len(inputMedia))

	if len(inputMedia) == 0 {
		log.Printf("%s No valid media found in messages. Skipping send.", logPrefix)
		return nil // Not an error, just nothing to send
	}

	// Send the media group using helper from helpers.go
	sentMessages, err := sendMediaGroupWithRetry(ctx, b.bot, b.channelID, b.debug, groupID, inputMedia, adminMediaGroupMaxSendRetries)
	if err != nil {
		log.Printf("%s Error sending group: %v", logPrefix, err)
		// Error is already captured by sentry inside sendMediaGroupWithRetry
		return fmt.Errorf("failed to send admin media group %s: %w", groupID, err)
	}

	// --- Post-Processing ---
	publishedTime := time.Now()
	channelPostID := 0
	if len(sentMessages) > 0 {
		channelPostID = sentMessages[0].MessageID // ID of the first message in the sent group
	}

	// Create Log Entry
	postLogEntry := &models.PostLog{
		SenderID:             firstMessage.From.ID,
		SenderUsername:       firstMessage.From.Username,
		Caption:              caption,
		MessageType:          "media_group",
		ReceivedAt:           time.Unix(int64(firstMessage.Date), 0),
		PublishedAt:          publishedTime,
		ChannelID:            b.channelID,
		ChannelPostID:        channelPostID,
		OriginalMediaGroupID: groupID,
	}

	// Log Post
	if errLog := b.postLogger.LogPublishedPost(*postLogEntry); errLog != nil {
		log.Printf("%s Error logging post to database: %v", logPrefix, errLog)
		sentry.CaptureException(fmt.Errorf("%s failed to log post: %w", logPrefix, errLog))
		// Continue even if logging fails
	}

	// Update User and Log Action (if user info available)
	if firstMessage.From != nil {
		userID := firstMessage.From.ID
		if errUpdate := b.userRepo.UpdateUser(ctx, userID, firstMessage.From.Username, firstMessage.From.FirstName, firstMessage.From.LastName, true, "send_media_group"); errUpdate != nil {
			log.Printf("%s Failed to update user info after sending media group: %v", logPrefix, errUpdate)
			sentry.CaptureException(fmt.Errorf("%s failed to update user %d after media group: %w", logPrefix, userID, errUpdate))
		}

		actionDetails := map[string]interface{}{
			"chat_id":            firstMessage.Chat.ID,
			"media_group_id":     groupID,
			"message_count":      len(msgs),
			"channel_message_id": channelPostID,
		}
		if errLogAction := b.actionLogger.LogUserAction(userID, "send_media_group", actionDetails); errLogAction != nil {
			log.Printf("%s Failed to log send_media_group action: %v", logPrefix, errLogAction)
			sentry.CaptureException(fmt.Errorf("%s failed to log send_media_group action for user %d: %w", logPrefix, userID, errLogAction))
		}
	} else {
		log.Printf("%s Cannot perform post-processing: From field is nil in the first message.", logPrefix)
	}

	log.Printf("%s Successfully processed admin media group -> Channel Post ID: %d.", logPrefix, channelPostID)
	// TODO: Send confirmation to admin?
	return nil // Success
}

// --- Media Group Helpers (Moved to helpers.go) ---

// createInputMedia moved to helpers.go

// sendMediaGroupWithRetry moved to helpers.go

// parseRetryAfter moved to helpers.go
