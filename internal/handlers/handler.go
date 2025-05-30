package handlers

import (
	"context"
	"log"
	"sync"
	"vrcmemes-bot/internal/auth" // Import auth for AdminCheckerInterface
	"vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/database/models"
	telegoapi "vrcmemes-bot/pkg/telegoapi" // Import telegoapi for BotAPI

	"github.com/mymmrac/telego"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// Command represents a bot command, mapping the command string to its description and handler function.
type Command struct {
	Command     string                                                        // The command string (e.g., "start").
	Description string                                                        // A short description of the command for /help.
	Handler     func(context.Context, telegoapi.BotAPI, telego.Message) error // Use telegoapi.BotAPI
}

// MessageHandler handles incoming Telegram messages and callbacks.
// It orchestrates command handling, message processing, caption management,
// interaction with the database, and suggestion workflow.
type MessageHandler struct {
	channelID int64 // The ID of the target Telegram channel for posting memes.

	// waitingForCaption stores chat IDs where the bot is currently waiting for the next text message to be used as a caption.
	// Key: chatID (int64), Value: true (bool)
	waitingForCaption sync.Map
	// activeCaptions stores the currently active caption for each chat.
	// Key: chatID (int64), Value: caption (string)
	activeCaptions sync.Map
	// mediaGroupCaptions temporarily stores captions associated with a media group ID, usually set by a preceding text message.
	// Key: mediaGroupID (string), Value: caption (string)
	mediaGroupCaptions sync.Map

	// commands holds the list of available bot commands.
	commands []Command

	version string // Added version field

	// Dependencies for database interactions and suggestion management.
	postLogger        database.PostLogger         // Interface for logging published posts.
	actionLogger      database.UserActionLogger   // Interface for logging user actions.
	userRepo          database.UserRepository     // Interface for updating user information.
	suggestionManager SuggestionManagerInterface  // Use SuggestionManagerInterface
	adminChecker      auth.AdminCheckerInterface  // Use auth.AdminCheckerInterface
	feedbackRepo      database.FeedbackRepository // Interface for saving feedback
}

// NewMessageHandler creates and initializes a new MessageHandler instance.
// It sets up dependencies and defines the available bot commands.
func NewMessageHandler(
	channelID int64,
	postLogger database.PostLogger,
	actionLogger database.UserActionLogger,
	userRepo database.UserRepository,
	suggestionManager SuggestionManagerInterface, // Use interface
	adminChecker auth.AdminCheckerInterface, // Use auth.AdminCheckerInterface
	feedbackRepo database.FeedbackRepository, // Accept FeedbackRepository
	version string, // Added version parameter
) *MessageHandler {
	if adminChecker == nil {
		// If AdminChecker is essential, consider logging a fatal error or returning an error
		log.Fatal("MessageHandler: Admin checker dependency is nil")
	}
	if suggestionManager == nil {
		log.Fatal("MessageHandler: Suggestion manager dependency is nil")
	}
	if feedbackRepo == nil {
		log.Fatal("MessageHandler: Feedback repository dependency is nil")
	}
	h := &MessageHandler{
		channelID:         channelID,
		postLogger:        postLogger,
		actionLogger:      actionLogger,
		userRepo:          userRepo,
		suggestionManager: suggestionManager,
		adminChecker:      adminChecker,
		feedbackRepo:      feedbackRepo,
		version:           version, // Assign version
	}
	// Initialize commands - Handler signatures already use telegoapi.BotAPI
	h.commands = []Command{
		{Command: "start", Description: "CmdStartDesc", Handler: h.HandleStart},
		{Command: "help", Description: "CmdHelpDesc", Handler: h.HandleHelp},
		{Command: "status", Description: "CmdStatusDesc", Handler: h.HandleStatus},
		{Command: "version", Description: "CmdVersionDesc", Handler: h.HandleVersion},
		{Command: "caption", Description: "CmdCaptionDesc", Handler: h.HandleCaption},
		{Command: "showcaption", Description: "CmdShowCaptionDesc", Handler: h.HandleShowCaption},
		{Command: "clearcaption", Description: "CmdClearCaptionDesc", Handler: h.HandleClearCaption},
		{Command: "suggest", Description: "CmdSuggestDesc", Handler: h.HandleSuggest},
		{Command: "review", Description: "CmdReviewDesc", Handler: h.HandleReview},
		{Command: "feedback", Description: "CmdFeedbackDesc", Handler: h.HandleFeedback},
		// TODO: Add other admin commands here if needed
	}
	return h
}

// GetChannelID returns the target channel ID configured for this handler.
func (h *MessageHandler) GetChannelID() int64 {
	return h.channelID
}

// GetCommandHandler retrieves the handler function associated with a specific command string (e.g., "start").
// It returns nil if the command is not found.
func (h *MessageHandler) GetCommandHandler(command string) func(context.Context, telegoapi.BotAPI, telego.Message) error { // Use telegoapi.BotAPI
	for _, cmd := range h.commands {
		if cmd.Command == command {
			return cmd.Handler
		}
	}
	return nil
}

// LogPublishedPost is a convenience method that wraps the call to the underlying postLogger.
func (h *MessageHandler) LogPublishedPost(logEntry models.PostLog) error {
	return h.postLogger.LogPublishedPost(logEntry)
}

// RetrieveMediaGroupCaption gets the caption stored temporarily for a given media group ID.
// It returns an empty string if no caption is found.
// Note: Consider if the caption should be deleted after retrieval.
func (h *MessageHandler) RetrieveMediaGroupCaption(groupID string) string {
	if caption, ok := h.mediaGroupCaptions.Load(groupID); ok {
		if capStr, okStr := caption.(string); okStr {
			// Optionally delete the caption after retrieval?
			// h.mediaGroupCaptions.Delete(groupID)
			return capStr
		}
	}
	return ""
}

// DeleteMediaGroupCaption removes the temporarily stored caption for a given media group ID.
func (h *MessageHandler) DeleteMediaGroupCaption(groupID string) {
	h.mediaGroupCaptions.Delete(groupID)
}

// UserRepo provides access to the user repository dependency.
func (h *MessageHandler) UserRepo() database.UserRepository {
	return h.userRepo
}

// ActionLogger provides access to the user action logger dependency.
func (h *MessageHandler) ActionLogger() database.UserActionLogger {
	return h.actionLogger
}

// ProcessSuggestionCallback delegates the handling of incoming callback queries
// (e.g., from inline keyboard buttons) to the suggestion manager.
// It returns true if the manager processed the callback, false otherwise.
func (h *MessageHandler) ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (bool, error) {
	if h.suggestionManager != nil {
		return h.suggestionManager.HandleCallbackQuery(ctx, query)
	}
	log.Println("Warning: ProcessSuggestionCallback called but suggestion manager is nil")
	return false, nil // Not processed
}

// SuggestionManager provides access to the suggestion manager dependency.
func (h *MessageHandler) SuggestionManager() SuggestionManagerInterface {
	return h.suggestionManager
}

// AdminChecker provides access to the admin checker dependency.
func (h *MessageHandler) AdminChecker() auth.AdminCheckerInterface {
	return h.adminChecker
}

// GetLocalizer provides access to the getLocalizer helper method (if needed externally).
// For internal use, call h.getLocalizer directly.
func (h *MessageHandler) GetLocalizer(user *telego.User) *i18n.Localizer { // Ensure i18n is imported in helpers.go
	return h.getLocalizer(user)
}
