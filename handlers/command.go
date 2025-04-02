package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	// "time" // time is not used directly in this file after logger refactoring

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // th is no longer needed
	// Assuming config.Version is needed
	"vrcmemes-bot/pkg/locales" // Import locales package
)

// HandleStart handles the /start command
func (h *MessageHandler) HandleStart(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	if err := h.setupCommands(ctx, bot); err != nil {
		return h.sendError(ctx, bot, message.Chat.ID, err)
	}

	// Update user information
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID) // isUserAdmin will also need to be updated
	isAdmin := false // Placeholder, need to update isUserAdmin
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_start")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log start command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_start", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		log.Printf("Failed to log start command: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, locales.MsgStart)
}

// HandleHelp handles the /help command
func (h *MessageHandler) HandleHelp(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	var helpText string
	for _, cmd := range h.commands {
		helpText += fmt.Sprintf("/%s - %s\n", cmd.Command, cmd.Description)
	}
	helpText += locales.MsgHelpFooter

	// Update user information
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_help")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log help command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_help", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		log.Printf("Failed to log help command: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, helpText)
}

// HandleStatus handles the /status command
func (h *MessageHandler) HandleStatus(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID)
	statusText := fmt.Sprintf(locales.MsgStatus, h.channelID, caption)

	// Update user information
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_status")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log status command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_status", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"caption": caption,
	})
	if err != nil {
		log.Printf("Failed to log status command: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command
func (h *MessageHandler) HandleVersion(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}
	versionText := fmt.Sprintf(locales.MsgVersion, version)

	// Update user information
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_version")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log version command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_version", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"version": version,
	})
	if err != nil {
		log.Printf("Failed to log version command: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, versionText)
}

// HandleCaption handles the /caption command
// ... existing code ...

// HandleShowCaption handles the /showcaption command
// ... existing code ...

// HandleClearCaption handles the /clearcaption command
// ... existing code ...

// HandleSuggest handles the /suggest command by calling the suggestion manager.
func (h *MessageHandler) HandleSuggest(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// We need the full Update object for the manager's handler
	// Construct a minimal Update containing the Message
	update := telego.Update{Message: &message}

	// Pass context and the constructed update to the manager
	err := h.suggestionManager.HandleSuggestCommand(ctx, update)
	if err != nil {
		// The manager handles sending messages to the user on errors like not subscribed.
		// We just log the error here if one occurs during the process.
		log.Printf("[HandleSuggest] Error from suggestionManager.HandleSuggestCommand for user %d: %v", message.From.ID, err)
		// Optionally, send a generic error message if the manager didn't?
		// For now, assume manager handles user feedback on errors.
	}
	return nil // Return nil to prevent generic error message from bot loop
}

// setupCommands registers bot commands
func (h *MessageHandler) setupCommands(ctx context.Context, bot *telego.Bot) error {
	commands := make([]telego.BotCommand, len(h.commands))
	for i, cmd := range h.commands {
		commands[i] = telego.BotCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		}
	}

	// Use the passed bot instance
	return bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: commands,
	})
}
