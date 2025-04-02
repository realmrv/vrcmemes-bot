package handlers

import (
	"context"
	"errors"
	"log"
	"sync"

	"vrcmemes-bot/database"
	"vrcmemes-bot/database/models"
	"vrcmemes-bot/internal/suggestions"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
)

// Command represents a bot command, mapping the command string to its description and handler function.
type Command struct {
	Command     string                                                   // The command string (e.g., "start").
	Description string                                                   // A short description of the command for /help.
	Handler     func(context.Context, *telego.Bot, telego.Message) error // The function to execute when the command is received.
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

	// Dependencies for database interactions and suggestion management.
	postLogger        database.PostLogger       // Interface for logging published posts.
	actionLogger      database.UserActionLogger // Interface for logging user actions.
	userRepo          database.UserRepository   // Interface for updating user information.
	suggestionManager *suggestions.Manager      // Manages the meme suggestion workflow.
}

// NewMessageHandler creates and initializes a new MessageHandler instance.
// It sets up dependencies and defines the available bot commands.
func NewMessageHandler(
	channelID int64,
	postLogger database.PostLogger,
	actionLogger database.UserActionLogger,
	userRepo database.UserRepository,
	suggestionManager *suggestions.Manager,
) *MessageHandler {
	h := &MessageHandler{
		channelID:         channelID,
		postLogger:        postLogger,
		actionLogger:      actionLogger,
		userRepo:          userRepo,
		suggestionManager: suggestionManager,
	}
	// Initialize commands using method values directly
	h.commands = []Command{
		{Command: "start", Description: cmdStartDesc, Handler: h.HandleStart},
		{Command: "help", Description: cmdHelpDesc, Handler: h.HandleHelp},
		{Command: "status", Description: cmdStatusDesc, Handler: h.HandleStatus},
		{Command: "version", Description: cmdVersionDesc, Handler: h.HandleVersion},
		{Command: "caption", Description: cmdCaptionDesc, Handler: h.HandleCaption},
		{Command: "showcaption", Description: cmdShowCaptionDesc, Handler: h.HandleShowCaption},
		{Command: "clearcaption", Description: cmdClearCaptionDesc, Handler: h.HandleClearCaption},
		{Command: "suggest", Description: cmdSuggestDesc, Handler: h.HandleSuggest},
		{Command: "review", Description: CmdReviewDesc, Handler: h.HandleReview},
	}
	return h
}

// GetChannelID returns the target channel ID configured for this handler.
func (h *MessageHandler) GetChannelID() int64 {
	return h.channelID
}

// GetCommandHandler retrieves the handler function associated with a specific command string (e.g., "start").
// It returns nil if the command is not found.
func (h *MessageHandler) GetCommandHandler(command string) func(context.Context, *telego.Bot, telego.Message) error {
	for _, cmd := range h.commands {
		if cmd.Command == command {
			return cmd.Handler
		}
	}
	return nil // Return nil if command not found
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

// HandleReview handles the /review command by delegating to the suggestion manager.
// This command likely initiates the process for reviewing pending suggestions.
func (h *MessageHandler) HandleReview(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// TODO: Implement review command logic by calling suggestionManager
	update := telego.Update{Message: &message} // Construct update for manager
	if h.suggestionManager != nil {
		return h.suggestionManager.HandleReviewCommand(ctx, update)
	} else {
		log.Println("Error: Suggestion manager not initialized in MessageHandler")
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(locales.MsgErrorGeneral))
	}
}

// UserRepo provides access to the user repository dependency.
func (h *MessageHandler) UserRepo() database.UserRepository {
	return h.userRepo
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

/* // Removed duplicate PostLog definition
type PostLog struct {
	SenderID             int64     `bson:"sender_id"`
	SenderUsername       string    `bson:"sender_username"`
	Caption              string    `bson:"caption"`
	MessageType          string    `bson:"message_type"`
	ReceivedAt           time.Time `bson:"received_at"`
	PublishedAt          time.Time `bson:"published_at"`
	ChannelID            int64     `bson:"channel_id"`
	ChannelPostID        int       `bson:"channel_post_id"`
	OriginalMediaGroupID string    `bson:"original_media_group_id"`
}
*/
