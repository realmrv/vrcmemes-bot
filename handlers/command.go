package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx *th.Context, message telego.Message) error {
	if err := h.setupCommands(ctx); err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	// Логируем команду /start
	_, err := h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "command_start",
		"details": map[string]interface{}{
			"chat_id": message.Chat.ID,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log start command: %v", err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, msgStart)
}

// HandleHelp handles the /help command
func (h *MessageHandler) HandleHelp(ctx *th.Context, message telego.Message) error {
	var helpText string
	for _, cmd := range h.commands {
		helpText += fmt.Sprintf("/%s - %s\n", cmd.Command, cmd.Description)
	}
	helpText += msgHelpFooter

	// Логируем команду /help
	_, err := h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "command_help",
		"details": map[string]interface{}{
			"chat_id": message.Chat.ID,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log help command: %v", err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, helpText)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx *th.Context, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID)
	statusText := fmt.Sprintf("Bot is running\nChannel ID: %d\nCaption: %s", h.channelID, caption)

	// Логируем команду /status
	_, err := h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "command_status",
		"details": map[string]interface{}{
			"chat_id": message.Chat.ID,
			"caption": caption,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log status command: %v", err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx *th.Context, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}

	// Логируем команду /version
	_, err := h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "command_version",
		"details": map[string]interface{}{
			"chat_id": message.Chat.ID,
			"version": version,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log version command: %v", err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, "Bot version: "+version)
}

// setupCommands registers bot commands
func (h *MessageHandler) setupCommands(ctx *th.Context) error {
	commands := make([]telego.BotCommand, len(h.commands))
	for i, cmd := range h.commands {
		commands[i] = telego.BotCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		}
	}

	return ctx.Bot().SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: commands,
	})
}
