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

// Command represents a bot command
type Command struct {
	Command     string
	Description string
	Handler     func(context.Context, *telego.Bot, telego.Message) error
}

// MessageHandler handles incoming messages
type MessageHandler struct {
	channelID int64
	// Map to store users waiting for captions
	waitingForCaption sync.Map
	// Map to store active captions for users
	activeCaptions sync.Map
	// Map to store captions for media groups
	mediaGroupCaptions sync.Map
	// Available commands
	commands          []Command
	postLogger        database.PostLogger
	actionLogger      database.UserActionLogger
	userRepo          database.UserRepository
	suggestionManager *suggestions.Manager
}

// NewMessageHandler creates a new message handler
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

// GetChannelID returns the configured channel ID.
func (h *MessageHandler) GetChannelID() int64 {
	return h.channelID
}

// GetCommandHandler returns the handler function for a given command string.
func (h *MessageHandler) GetCommandHandler(command string) func(context.Context, *telego.Bot, telego.Message) error {
	for _, cmd := range h.commands {
		if cmd.Command == command {
			return cmd.Handler
		}
	}
	return nil // Return nil if command not found
}

// LogPublishedPost wraps the postLogger call.
func (h *MessageHandler) LogPublishedPost(logEntry models.PostLog) error {
	return h.postLogger.LogPublishedPost(logEntry)
}

// RetrieveMediaGroupCaption retrieves a stored caption for a media group.
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

// DeleteMediaGroupCaption removes a stored caption for a media group.
func (h *MessageHandler) DeleteMediaGroupCaption(groupID string) {
	h.mediaGroupCaptions.Delete(groupID)
}

// HandleReview handles the /review command (placeholder)
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

// UserRepo returns the user repository instance.
func (h *MessageHandler) UserRepo() database.UserRepository {
	return h.userRepo
}

// ProcessSuggestionCallback delegates callback query handling to the suggestion manager.
// Returns true if the callback was processed, false otherwise.
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
