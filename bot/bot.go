package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	dbi "vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/database/models" // Import models
	"vrcmemes-bot/internal/handlers"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/internal/mediagroups"
	"vrcmemes-bot/internal/suggestions"
	telegoapi "vrcmemes-bot/pkg/telegoapi"

	"github.com/getsentry/sentry-go"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.uber.org/ratelimit"
)

// Bot represents the main application logic for the Telegram bot.
// It wraps the telego library, manages the update loop, handles different update types,
// and orchestrates complex operations like media group processing.
type Bot struct {
	bot           telegoapi.BotAPI // Use BotAPI interface
	updatesChan   <-chan telego.Update
	mediaGroups   sync.Map
	debug         bool
	channelID     int64
	captionProv   dbi.CaptionProvider
	postLogger    dbi.PostLogger
	handerProv    dbi.HandlerProvider                 // Interface uses telegoapi.BotAPI now
	suggestionMgr handlers.SuggestionManagerInterface // Use handlers interface
	callbackProc  dbi.CallbackProcessor               // Still needed?
	userRepo      dbi.UserRepository
	actionLogger  dbi.UserActionLogger
	mediaGroupMgr *mediagroups.Manager
	handler       *handlers.MessageHandler
	ratelimiter   ratelimit.Limiter
}

// BotDeps holds the dependencies required by the Bot.
type BotDeps struct {
	Bot           telegoapi.BotAPI     // Use BotAPI interface
	UpdatesChan   <-chan telego.Update // Receive the channel
	Debug         bool
	ChannelID     int64
	CaptionProv   dbi.CaptionProvider
	PostLogger    dbi.PostLogger
	HandlerProv   dbi.HandlerProvider                 // Interface uses telegoapi.BotAPI
	SuggestionMgr handlers.SuggestionManagerInterface // Use handlers interface
	CallbackProc  dbi.CallbackProcessor               // Still needed?
	UserRepo      dbi.UserRepository
	ActionLogger  dbi.UserActionLogger
	MediaGroupMgr *mediagroups.Manager
	Handler       *handlers.MessageHandler
}

// New creates a new Bot instance from its dependencies.
// Returns the new Bot instance or an error if dependencies are missing.
func New(deps BotDeps) (*Bot, error) {
	// Validate dependencies
	if deps.Bot == nil {
		return nil, fmt.Errorf("telego bot (BotAPI) instance cannot be nil")
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
	if deps.Handler == nil { // Add check for Handler if it becomes essential
		return nil, fmt.Errorf("message handler cannot be nil")
	}
	if deps.UpdatesChan == nil {
		return nil, fmt.Errorf("updates channel cannot be nil") // Add check
	}

	return &Bot{
		bot:           deps.Bot,
		updatesChan:   deps.UpdatesChan, // Store the channel
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
		handler:       deps.Handler,
		ratelimiter:   ratelimit.New(20),
	}, nil
}

// handleCommandUpdate processes a message identified as a command.
func (b *Bot) handleCommandUpdate(ctx context.Context, message telego.Message) {
	command := "unknown"
	if len(message.Text) > 1 && strings.HasPrefix(message.Text, "/") {
		command = strings.Split(message.Text, " ")[0][1:] // Extract command without leading slash
	}
	logPrefix := fmt.Sprintf("[Cmd:%s User:%d]", command, message.From.ID)

	handlerFunc := b.handerProv.GetCommandHandler(command) // Returns func(..., telegoapi.BotAPI, ...)
	if handlerFunc != nil {
		if b.debug {
			log.Printf("%s Executing handler", logPrefix)
		}
		// Pass b.bot (which is telegoapi.BotAPI) to the handler
		err := handlerFunc(ctx, b.bot, message)
		if err != nil {
			log.Printf("%s Handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		} else {
			if b.debug {
				log.Printf("%s Handler finished successfully", logPrefix)
			}
		}
	} else {
		log.Printf("%s No handler found", logPrefix)
		// Send localized "unknown command" message
		lang := locales.GetDefaultLanguageTag().String() // Use tag
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
		// Pass b.bot (telegoapi.BotAPI) to HandlePhoto
		err := b.handerProv.HandlePhoto(ctx, b.bot, message)
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
		// Pass b.bot (telegoapi.BotAPI) to HandleText
		err := b.handerProv.HandleText(ctx, b.bot, message)
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
		// Pass b.bot (telegoapi.BotAPI) to HandleVideo
		err := b.handerProv.HandleVideo(ctx, b.bot, message)
		if err != nil {
			log.Printf("%s Handler error: %v", logPrefix, err)
			sentry.CaptureException(fmt.Errorf("%s handler error: %w", logPrefix, err))
		}
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
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String()) // Use tag

	// Delegate to suggestion manager
	processed, err := b.suggestionMgr.HandleCallbackQuery(ctx, query)
	if err != nil {
		log.Printf("%s Suggestion callback handler error: %v", logPrefix, err)
		sentry.CaptureException(fmt.Errorf("%s suggestion callback handler error: %w", logPrefix, err))
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: errorMsg})
		return
	}

	if processed {
		if b.debug {
			log.Printf("%s Callback handled by suggestion manager", logPrefix)
		}
		// Suggestion manager should answer the query.
		return
	}

	// If not processed by suggestion manager, maybe log or answer with default?
	log.Printf("%s Callback query not handled", logPrefix)
	defaultAnswer := locales.GetMessage(localizer, "MsgCallbackNotHandled", nil, nil) // Assuming this key exists
	_ = b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: defaultAnswer, ShowAlert: true})
}

// processUpdate routes incoming updates to the appropriate handlers.
func (b *Bot) processUpdate(ctx context.Context, update telego.Update) {
	// Apply global rate limiting
	b.ratelimiter.Take()

	// Handle potential panics in handlers
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC recovered in processUpdate: %v\n%s", r, debug.Stack())
			sentry.CurrentHub().Recover(r)
			sentry.Flush(time.Second * 2)
		}
	}()

	// Create a context with timeout for the update processing
	processingCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // 30 seconds timeout for processing
	defer cancel()

	switch {
	case update.Message != nil:
		message := *update.Message
		if message.From == nil { // Ignore messages without a sender (e.g., channel posts from linked chat)
			log.Printf("Ignoring message %d from chat %d without sender", message.MessageID, message.Chat.ID)
			return
		}

		// Update user info and log action (Consider moving this inside specific handlers if needed)
		// isAdminCheckNeeded := true // Or determine based on message type
		// isAdmin := false
		// if isAdminCheckNeeded { ... isAdmin, _ = b.handler.AdminChecker().IsAdmin ... }
		// b.actionLogger.LogUserAction(...) // Action type depends on message
		// b.userRepo.UpdateUser(...) // Update user info

		// 1. Handle Media Groups via MediaGroupManager
		if message.MediaGroupID != "" {
			// Pass message, not update, to HandleMessage
			// Add default delay and size arguments
			err := b.mediaGroupMgr.HandleMessage(
				processingCtx,
				message,
				b.handleCombinedMediaGroup,
				mediagroups.DefaultProcessDelay, // Add delay
				mediagroups.DefaultMaxGroupSize, // Add max size
			)
			if err != nil {
				log.Printf("Error handling media group %s via manager: %v", message.MediaGroupID, err)
			}
			return
		}

		// 2. Process Suggestions/Feedback via Suggestion Manager
		processed, err := b.suggestionMgr.HandleMessage(processingCtx, update)
		if err != nil {
			log.Printf("Error processing message %d via suggestion manager: %v", message.MessageID, err)
			// Manager should send feedback
			return
		}
		if processed {
			if b.debug {
				log.Printf("Message %d processed by suggestion manager", message.MessageID)
			}
			return
		}

		// 3. Handle Commands, Photos, Videos, Text (if not handled above)
		if strings.HasPrefix(message.Text, "/") {
			b.handleCommandUpdate(processingCtx, message)
		} else if message.Photo != nil {
			b.handlePhotoUpdate(processingCtx, message)
		} else if message.Video != nil {
			b.handleVideoUpdate(processingCtx, message)
		} else if message.Text != "" {
			b.handleTextUpdate(processingCtx, message)
		} else {
			if b.debug {
				log.Printf("Ignoring unhandled message type (ID: %d)", message.MessageID)
			}
		}

	case update.CallbackQuery != nil:
		b.handleCallbackQuery(processingCtx, *update.CallbackQuery)

	default:
		if b.debug {
			log.Printf("Ignoring unhandled update type: %+v", update)
		}
	}
}

// Start begins the bot's update processing loop.
// It now uses the updatesChan passed during initialization.
func (b *Bot) Start(ctx context.Context) {
	if b.updatesChan == nil {
		log.Fatal("Bot updates channel is nil, cannot start")
	}
	log.Println("Listening for updates...")

	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			log.Println("Context done, stopping update processing...")
			wg.Wait() // Wait for all processing goroutines to finish
			log.Println("All update processing finished.")
			return
		case update, ok := <-b.updatesChan: // Read from the stored channel
			if !ok {
				log.Println("Updates channel closed.")
				wg.Wait() // Ensure processing finishes if channel closes unexpectedly
				return
			}
			wg.Add(1)
			go func(up telego.Update) {
				defer wg.Done()
				b.processUpdate(ctx, up)
			}(update)
		}
	}
}

// Stop gracefully stops the bot.
// No longer needs to interact with telego's Stop/Delete methods directly.
func (b *Bot) Stop() {
	log.Println("Bot Stop method called. Actual stop triggered by context cancellation.")
	// Add any other specific cleanup needed by the Bot struct itself
}

// handleCombinedMediaGroup is the handler passed to the MediaGroupManager.
func (b *Bot) handleCombinedMediaGroup(ctx context.Context, groupID string, messages []telego.Message) error {
	// ... (logic remains mostly the same, ensure it uses b.suggestionMgr which is SuggestionManagerInterface)
	if len(messages) == 0 {
		return errors.New("received empty media group")
	}
	firstMessage := messages[0]
	userID := firstMessage.From.ID
	chatID := firstMessage.Chat.ID
	log.Printf("[MediaGroupHandler] Processing group %s from User %d in Chat %d (%d messages)", groupID, userID, chatID, len(messages))

	userState := b.suggestionMgr.GetUserState(userID)

	if userState == suggestions.StateAwaitingSuggestion || userState == suggestions.StateAwaitingFeedback {
		log.Printf("[MediaGroupHandler Group:%s] Delegating to SuggestionManager (UserState: %v)", groupID, userState)
		// Use the suggestion manager interface method
		return b.suggestionMgr.HandleCombinedMediaGroup(ctx, groupID, messages)
	} else {
		log.Printf("[MediaGroupHandler Group:%s] Handling as potential Admin post (UserState: %v)", groupID, userState)
		return b.handleAdminMediaGroup(ctx, groupID, messages)
	}
}

// handleAdminMediaGroup handles media groups sent directly by an admin.
func (b *Bot) handleAdminMediaGroup(ctx context.Context, groupID string, messages []telego.Message) error {
	if len(messages) == 0 {
		return errors.New("received empty admin media group")
	}
	firstMessage := messages[0]
	userID := firstMessage.From.ID
	chatID := firstMessage.Chat.ID
	localizer := b.handler.GetLocalizer(firstMessage.From) // Get localizer via handler

	if b.handler == nil {
		return errors.New("internal error: message handler not available for admin check")
	}
	isAdmin, err := b.handler.AdminChecker().IsAdmin(ctx, userID) // Use AdminChecker() method
	if err != nil {
		log.Printf("[AdminMediaGroup] Error checking admin status for user %d: %v", userID, err)
		// Don't reveal internal errors, maybe send generic denial? For now, just return error.
		return fmt.Errorf("admin check failed: %w", err)
	}
	if !isAdmin {
		log.Printf("[AdminMediaGroup] Non-admin user %d attempted to send media group directly.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		_, _ = b.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return nil // Not an error, just denied
	}

	// Retrieve caption
	caption := b.handler.RetrieveMediaGroupCaption(groupID)
	if caption != "" {
		b.handler.DeleteMediaGroupCaption(groupID)
	} else {
		activeCaption, exists := b.handler.GetActiveCaption(chatID)
		if exists {
			caption = activeCaption
		}
	}

	// Prepare media
	media := make([]telego.InputMedia, 0, len(messages))
	for i, msg := range messages {
		var input telego.InputMedia
		if msg.Photo != nil {
			inputFile := telego.InputFile{FileID: msg.Photo[len(msg.Photo)-1].FileID}
			mediaPhoto := tu.MediaPhoto(inputFile)
			if i == 0 {
				mediaPhoto.Caption = caption // Set caption directly
			}
			input = mediaPhoto
		} else if msg.Video != nil {
			inputFile := telego.InputFile{FileID: msg.Video.FileID}
			mediaVideo := tu.MediaVideo(inputFile)
			if i == 0 {
				mediaVideo.Caption = caption // Set caption directly
			}
			input = mediaVideo
		} else {
			log.Printf("[AdminMediaGroup] Unsupported media type in group %s from user %d, skipping message %d", groupID, userID, msg.MessageID)
			continue
		}
		media = append(media, input)
	}
	if len(media) == 0 {
		log.Printf("[AdminMediaGroup] No valid media found in group %s from user %d after filtering.", groupID, userID)
		return nil // No media to send
	}

	// Send media group using b.bot
	sentMessages, err := b.bot.SendMediaGroup(ctx, tu.MediaGroup(tu.ID(b.handler.GetChannelID()), media...))
	if err != nil {
		log.Printf("[AdminMediaGroup] Failed to send media group %s: %v", groupID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorSendToChannel", nil, nil)
		_, _ = b.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		return err
	}

	// Log post using b.handler.LogPublishedPost
	publishedTime := time.Now()
	channelMessageID := 0
	if len(sentMessages) > 0 {
		channelMessageID = sentMessages[0].MessageID
	}
	logEntry := models.PostLog{
		SenderID:             userID,
		SenderUsername:       firstMessage.From.Username,
		Caption:              caption,
		MessageType:          "media_group",
		ReceivedAt:           time.Unix(int64(firstMessage.Date), 0),
		PublishedAt:          publishedTime,
		ChannelID:            b.handler.GetChannelID(),
		ChannelPostID:        channelMessageID,
		OriginalMediaGroupID: groupID,
	}
	if err := b.handler.LogPublishedPost(logEntry); err != nil {
		log.Printf("Error logging admin media group post for group %s: %v", groupID, err)
	}

	// Record activity using b.handler.RecordUserActivity (assuming firstMessage is defined)
	b.handler.RecordUserActivity(ctx, firstMessage.From, "send_media_group_to_channel", isAdmin, map[string]interface{}{
		"chat_id":            chatID,
		"media_group_id":     groupID,
		"message_count":      len(messages),
		"channel_message_id": channelMessageID,
		"caption_used":       caption,
	})

	// Send confirmation using b.bot
	confirmationMsg := locales.GetMessage(localizer, "MsgPostSentToChannel", nil, nil)
	_, _ = b.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))

	return nil
}

func (b *Bot) setupCommands(ctx context.Context) error {
	// Get commands from handler provider
	// Assuming GetCommandHandler("") returns metadata for all commands or similar
	// This logic needs clarification based on HandlerProvider implementation.
	// For now, manually define commands:

	defaultLang := locales.GetDefaultLanguageTag().String() // Use tag
	localizer := locales.NewLocalizer(defaultLang)

	cmds := []telego.BotCommand{
		{
			Command:     "start",
			Description: locales.GetMessage(localizer, "CmdStartDescription", nil, nil),
		},
		{
			Command:     "help",
			Description: locales.GetMessage(localizer, "CmdHelpDescription", nil, nil),
		},
		{
			Command:     "settings",
			Description: locales.GetMessage(localizer, "CmdSettingsDescription", nil, nil),
		},
	}

	// TODO: Add other commands as needed (feedback, stats, etc.)

	// Set the commands using the correct parameters struct
	params := &telego.SetMyCommandsParams{
		Commands: cmds,
		// Scope: telego.BotCommandScopeDefault{}, // Default scope
		// LanguageCode: "", // Default language
	}
	err := b.bot.SetMyCommands(ctx, params) // Correct call, returns only error
	if err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	log.Println("Bot commands successfully set.")
	return nil
}
