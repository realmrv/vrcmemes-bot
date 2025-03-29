package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	version = "1.0.0"
)

// Message types
const (
	msgStart               = "Hello! I'm a bot for creating posts in the channel. Send me the text you want to publish.\n\nUse the menu below to see available commands."
	msgCaptionPrompt       = "Please enter the caption text for your next photos. This will replace any existing caption."
	msgCaptionSaved        = "Caption saved! All photos you send will use this caption. Use /caption again to change it."
	msgCaptionOverwrite    = "Previous caption has been replaced with the new one."
	msgPostSuccess         = "Post successfully published in the channel!"
	msgPhotoSuccess        = "Photo successfully published in the channel!"
	msgPhotoWithCaption    = "Photo with caption successfully published in the channel!"
	msgHelpFooter          = "\nTo create a post, simply send any text message.\nTo add a caption to a photo, use /caption command and then send the photo."
	msgNoCaptionSet        = "No active caption set. Use /caption to set one."
	msgCurrentCaption      = "Current active caption:\n%s"
	msgCaptionCleared      = "Active caption has been cleared."
	msgErrorSendingMessage = "Error sending message: %s"
	msgErrorCopyingMessage = "Error copying message: %s"
)

// Command represents a bot command
type Command struct {
	Command     string
	Description string
	Handler     func(context.Context, *bot.Bot, *models.Update)
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
	h := &MessageHandler{
		channelID: channelID,
	}

	// Initialize commands
	h.commands = []Command{
		{Command: "start", Description: "Start the bot", Handler: h.HandleStart},
		{Command: "help", Description: "Show help message", Handler: h.HandleHelp},
		{Command: "status", Description: "Check bot status", Handler: h.HandleStatus},
		{Command: "version", Description: "Show bot version", Handler: h.HandleVersion},
		{Command: "caption", Description: "Set caption for next photos", Handler: h.HandleCaption},
		{Command: "showcaption", Description: "Show current active caption", Handler: h.HandleShowCaption},
		{Command: "clearcaption", Description: "Clear current active caption", Handler: h.HandleClearCaption},
	}

	return h
}

// sendError sends an error message to the user
func (h *MessageHandler) sendError(ctx context.Context, b *bot.Bot, chatID int64, err error) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf(msgErrorSendingMessage, err.Error()),
	})
}

// sendSuccess sends a success message to the user
func (h *MessageHandler) sendSuccess(ctx context.Context, b *bot.Bot, chatID int64, message string) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   message,
	})
}

// setupCommands registers bot commands
func (h *MessageHandler) setupCommands(ctx context.Context, b *bot.Bot) error {
	commands := make([]models.BotCommand, len(h.commands))
	for i, cmd := range h.commands {
		commands[i] = models.BotCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		}
	}

	_, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: commands,
	})
	return err
}

// validateMessage checks if the message is valid
func (h *MessageHandler) validateMessage(update *models.Update) bool {
	return update != nil && update.Message != nil
}

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	if err := h.setupCommands(ctx, b); err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, msgStart)
}

// HandleHelp handles the /help command
func (h *MessageHandler) HandleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	var helpText string
	for _, cmd := range h.commands {
		helpText += fmt.Sprintf("/%s - %s\n", cmd.Command, cmd.Description)
	}
	helpText += msgHelpFooter

	h.sendSuccess(ctx, b, update.Message.Chat.ID, helpText)
}

// HandleCaption handles the /caption command
func (h *MessageHandler) HandleCaption(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	h.waitingForCaption.Store(update.Message.Chat.ID, true)
	h.sendSuccess(ctx, b, update.Message.Chat.ID, msgCaptionPrompt)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	statusText := fmt.Sprintf("Bot is running\nChannel ID: %d", h.channelID)
	h.sendSuccess(ctx, b, update.Message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, "Bot version: "+version)
}

// getActiveCaption returns the active caption for a user
func (h *MessageHandler) getActiveCaption(chatID int64) (string, bool) {
	if caption, exists := h.activeCaptions.Load(chatID); exists {
		return caption.(string), true
	}
	return "", false
}

// setActiveCaption sets the active caption for a user
func (h *MessageHandler) setActiveCaption(chatID int64, caption string) {
	h.activeCaptions.Store(chatID, caption)
}

// clearActiveCaption removes the active caption for a user
func (h *MessageHandler) clearActiveCaption(chatID int64) {
	h.activeCaptions.Delete(chatID)
}

// HandleText handles text messages
func (h *MessageHandler) HandleText(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) || update.Message.Text == "" || update.Message.Text == "/start" {
		return
	}

	if _, waiting := h.waitingForCaption.Load(update.Message.Chat.ID); waiting {
		// Check if there was a previous caption
		_, hadPreviousCaption := h.getActiveCaption(update.Message.Chat.ID)

		// Store the new caption for future photos
		h.setActiveCaption(update.Message.Chat.ID, update.Message.Text)
		h.waitingForCaption.Delete(update.Message.Chat.ID)

		// Send appropriate message
		if hadPreviousCaption {
			h.sendSuccess(ctx, b, update.Message.Chat.ID, msgCaptionOverwrite)
		} else {
			h.sendSuccess(ctx, b, update.Message.Chat.ID, msgCaptionSaved)
		}
		return
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.channelID,
		Text:   update.Message.Text,
	})
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	h.sendSuccess(ctx, b, update.Message.Chat.ID, msgPostSuccess)
}

// HandlePhoto handles photo messages
func (h *MessageHandler) HandlePhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) || update.Message.Photo == nil {
		return
	}

	// Handle single photo
	params := &bot.CopyMessageParams{
		ChatID:     h.channelID,
		FromChatID: update.Message.Chat.ID,
		MessageID:  update.Message.ID,
	}

	// Get active caption if exists
	if caption, exists := h.getActiveCaption(update.Message.Chat.ID); exists {
		params.Caption = caption
	}

	_, err := b.CopyMessage(ctx, params)
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	successMsg := msgPhotoSuccess
	if params.Caption != "" {
		successMsg = msgPhotoWithCaption
	}
	h.sendSuccess(ctx, b, update.Message.Chat.ID, successMsg)
}

// HandleMediaGroup handles media group messages
func (h *MessageHandler) HandleMediaGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) || update.Message.MediaGroupID == "" {
		return
	}

	// Get caption if exists
	var caption string
	if c, exists := h.mediaGroupCaptions.Load(update.Message.MediaGroupID); exists {
		caption = c.(string)
	} else if c, exists := h.getActiveCaption(update.Message.Chat.ID); exists {
		caption = c
		h.mediaGroupCaptions.Store(update.Message.MediaGroupID, caption)
	}

	// Copy the message with caption
	params := &bot.CopyMessageParams{
		ChatID:     h.channelID,
		FromChatID: update.Message.Chat.ID,
		MessageID:  update.Message.ID,
		Caption:    caption,
	}

	_, err := b.CopyMessage(ctx, params)
	if err != nil {
		h.sendError(ctx, b, update.Message.Chat.ID, err)
		return
	}

	successMsg := msgPhotoSuccess
	if caption != "" {
		successMsg = msgPhotoWithCaption
	}
	h.sendSuccess(ctx, b, update.Message.Chat.ID, successMsg)
}

// HandleShowCaption handles the /showcaption command
func (h *MessageHandler) HandleShowCaption(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	if caption, exists := h.getActiveCaption(update.Message.Chat.ID); exists {
		h.sendSuccess(ctx, b, update.Message.Chat.ID, fmt.Sprintf(msgCurrentCaption, caption))
	} else {
		h.sendSuccess(ctx, b, update.Message.Chat.ID, msgNoCaptionSet)
	}
}

// HandleClearCaption handles the /clearcaption command
func (h *MessageHandler) HandleClearCaption(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.validateMessage(update) {
		return
	}

	h.clearActiveCaption(update.Message.Chat.ID)
	h.sendSuccess(ctx, b, update.Message.Chat.ID, msgCaptionCleared)
}
