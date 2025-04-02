package handlers

import (
	"context"
	"sync"

	"vrcmemes-bot/database"
	"vrcmemes-bot/database/models"

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
	commands     []Command
	postLogger   database.PostLogger
	actionLogger database.UserActionLogger
	userRepo     database.UserRepository
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(channelID int64, postLogger database.PostLogger, actionLogger database.UserActionLogger, userRepo database.UserRepository) *MessageHandler {
	h := &MessageHandler{
		channelID:    channelID,
		postLogger:   postLogger,
		actionLogger: actionLogger,
		userRepo:     userRepo,
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
