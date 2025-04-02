package handlers

import (
	"sync"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.mongodb.org/mongo-driver/mongo"
)

// Command represents a bot command
type Command struct {
	Command     string
	Description string
	Handler     func(*MessageHandler, *th.Context, telego.Message) error
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
	commands []Command
	db       *mongo.Database
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(channelID int64, db *mongo.Database) *MessageHandler {
	return &MessageHandler{
		channelID: channelID,
		db:        db,
		commands: []Command{
			{Command: "start", Description: cmdStartDesc, Handler: (*MessageHandler).HandleStart},
			{Command: "help", Description: cmdHelpDesc, Handler: (*MessageHandler).HandleHelp},
			{Command: "status", Description: cmdStatusDesc, Handler: (*MessageHandler).HandleStatus},
			{Command: "version", Description: cmdVersionDesc, Handler: (*MessageHandler).HandleVersion},
			{Command: "caption", Description: cmdCaptionDesc, Handler: (*MessageHandler).HandleCaption},
			{Command: "showcaption", Description: cmdShowCaptionDesc, Handler: (*MessageHandler).HandleShowCaption},
			{Command: "clearcaption", Description: cmdClearCaptionDesc, Handler: (*MessageHandler).HandleClearCaption},
		},
	}
}

// PostLog represents the structure for logging published posts in MongoDB
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
