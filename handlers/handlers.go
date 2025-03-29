package handlers

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	version = "1.0.0"
)

// MessageHandler handles incoming messages
type MessageHandler struct {
	channelID int64
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(channelID int64) *MessageHandler {
	return &MessageHandler{
		channelID: channelID,
	}
}

// sendError sends an error message to the user
func (h *MessageHandler) sendError(ctx context.Context, b *bot.Bot, chatID int64, err error) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Error: " + err.Error(),
	})
}

// sendSuccess sends a success message to the user
func (h *MessageHandler) sendSuccess(ctx context.Context, b *bot.Bot, chatID int64, message string) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   message,
	})
}

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	// Set up commands menu
	commands := []models.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help message"},
		{Command: "status", Description: "Check bot status"},
		{Command: "version", Description: "Show bot version"},
	}

	_, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: commands,
	})
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID,
		"Hello! I'm a bot for creating posts in the channel. Send me the text you want to publish.\n\nUse the menu below to see available commands.")
}

// HandleHelp handles the /help command
func (h *MessageHandler) HandleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	helpText := `Available commands:
/start - Start the bot
/help - Show this help message
/status - Check bot status
/version - Show bot version

To create a post, simply send any text message.`

	h.sendSuccess(ctx, b, update.Message.Chat.ID, helpText)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	statusText := fmt.Sprintf("Bot is running\nChannel ID: %d", h.channelID)
	h.sendSuccess(ctx, b, update.Message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, "Bot version: "+version)
}

// HandleText handles text messages
func (h *MessageHandler) HandleText(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" || update.Message.Text == "/start" {
		return
	}

	// Creating post in the channel
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.channelID,
		Text:   update.Message.Text,
	})
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, "Post successfully published in the channel!")
}

// HandlePhoto handles photo messages
func (h *MessageHandler) HandlePhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Photo == nil {
		return
	}

	// Forward photo to channel
	_, err := b.ForwardMessage(ctx, &bot.ForwardMessageParams{
		ChatID:     h.channelID,
		FromChatID: update.Message.Chat.ID,
		MessageID:  update.Message.ID,
	})
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, "Photo successfully published in the channel!")
}
