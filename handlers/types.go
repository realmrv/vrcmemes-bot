package handlers

import (
	"sync"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
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
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(channelID int64) *MessageHandler {
	return &MessageHandler{
		channelID: channelID,
		commands: []Command{
			{Command: "start", Description: "Start the bot", Handler: (*MessageHandler).HandleStart},
			{Command: "help", Description: "Show help message", Handler: (*MessageHandler).HandleHelp},
			{Command: "status", Description: "Show bot status", Handler: (*MessageHandler).HandleStatus},
			{Command: "version", Description: "Show bot version", Handler: (*MessageHandler).HandleVersion},
			{Command: "caption", Description: "Set caption for photos", Handler: (*MessageHandler).HandleCaption},
			{Command: "showcaption", Description: "Show current caption", Handler: (*MessageHandler).HandleShowCaption},
			{Command: "clearcaption", Description: "Clear current caption", Handler: (*MessageHandler).HandleClearCaption},
		},
	}
}
