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

// HandleStart handles the /start command.
// It sets up the bot commands, updates user info, logs the action, and sends a welcome message.
func (h *MessageHandler) HandleStart(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	if err := h.setupCommands(ctx, bot); err != nil {
		// Add context before returning the error
		return h.sendError(ctx, bot, message.Chat.ID, fmt.Errorf("failed to set up commands: %w", err))
	}

	// Placeholder: Determine if the user is an admin (requires implementation)
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID) // isUserAdmin will also need to be updated
	isAdmin := false // Placeholder, need to update isUserAdmin
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_start")
	if err != nil {
		// Log internal error, don't return to user unless critical
		log.Printf("Failed to update user info for user %d during /start: %v", message.From.ID, err)
		// Potentially send to Sentry here
	}

	// Log start command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_start", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		// Log internal error
		log.Printf("Failed to log /start command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry here
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, locales.MsgStart)
}

// HandleHelp handles the /help command.
// It generates a help message listing available commands, updates user info, logs the action, and sends the help text.
func (h *MessageHandler) HandleHelp(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	var helpText string
	for _, cmd := range h.commands {
		helpText += fmt.Sprintf("/%s - %s\\n", cmd.Command, cmd.Description)
	}
	helpText += locales.MsgHelpFooter

	// Placeholder: Determine admin status
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_help")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /help: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log help command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_help", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		log.Printf("Failed to log /help command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, helpText)
}

// HandleStatus handles the /status command.
// It retrieves the current active caption, formats a status message, updates user info, logs the action, and sends the status.
func (h *MessageHandler) HandleStatus(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID) // Assuming GetActiveCaption handles potential errors or defaults gracefully
	statusText := fmt.Sprintf(locales.MsgStatus, h.channelID, caption)

	// Placeholder: Determine admin status
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_status")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /status: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log status command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_status", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"caption": caption,
	})
	if err != nil {
		log.Printf("Failed to log /status command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command.
// It retrieves the application version, formats a version message, updates user info, logs the action, and sends the version.
func (h *MessageHandler) HandleVersion(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev" // Default version if not set
	}
	versionText := fmt.Sprintf(locales.MsgVersion, version)

	// Placeholder: Determine admin status
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID)
	isAdmin := false // Placeholder
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_version")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /version: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log version command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_version", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"version": version,
	})
	if err != nil {
		log.Printf("Failed to log /version command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
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
// It constructs a telego.Update object and passes it to the suggestion manager's handler.
// Errors during suggestion handling are logged, assuming the manager handles user feedback.
func (h *MessageHandler) HandleSuggest(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// We need the full Update object for the manager's handler
	// Construct a minimal Update containing the Message
	update := telego.Update{Message: &message}

	// Delegate the handling to the suggestion manager
	err := h.suggestionManager.HandleSuggestCommand(ctx, update)
	if err != nil {
		// The manager is expected to handle sending messages to the user on errors
		// (e.g., user not subscribed, invalid format). We just log the error here
		// if one occurs during the manager's processing.
		log.Printf("[HandleSuggest] Error from suggestionManager.HandleSuggestCommand for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}
	// Return nil to prevent the main bot loop from sending a generic error message.
	// User feedback should be handled entirely by the suggestionManager.
	return nil
}

// setupCommands registers the bot's commands with Telegram.
// It builds the list of commands from the handler's configuration and uses the bot instance to set them.
func (h *MessageHandler) setupCommands(ctx context.Context, bot *telego.Bot) error {
	if len(h.commands) == 0 {
		log.Println("No commands defined in handler, skipping SetMyCommands.")
		return nil // No commands to set is not an error
	}

	commands := make([]telego.BotCommand, len(h.commands))
	for i, cmd := range h.commands {
		commands[i] = telego.BotCommand{
			Command:     cmd.Command,
			Description: cmd.Description,
		}
	}

	// Use the passed bot instance to set the commands
	err := bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: commands,
	})
	if err != nil {
		// Wrap the error with context before returning
		return fmt.Errorf("failed to set bot commands: %w", err)
	}
	log.Printf("Successfully set %d bot commands.", len(commands))
	return nil
}
