package handlers

import (
	"fmt"
	"os"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx *th.Context, message telego.Message) error {
	if err := h.setupCommands(ctx); err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
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
	return h.sendSuccess(ctx, message.Chat.ID, helpText)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx *th.Context, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID)
	statusText := fmt.Sprintf("Bot is running\nChannel ID: %d\nCaption: %s", h.channelID, caption)
	return h.sendSuccess(ctx, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx *th.Context, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
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
