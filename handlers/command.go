package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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
		// Consider adding more context to the error before sending
		// Maybe use locales for this error message too?
		return h.sendError(ctx, bot, message.Chat.ID, fmt.Errorf("failed to set up commands: %w", err))
	}

	// Create a localizer (determine user language later, default to Russian for now)
	// TODO: Detect user language from message.From.LanguageCode if available
	lang := locales.DefaultLanguage // Default to Russian
	if message.From != nil && message.From.LanguageCode != "" {
		// Potentially use user's language if supported
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Placeholder: Determine if the user is an admin (requires implementation)
	// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID) // isUserAdmin will also need to be updated
	isAdmin := false // Placeholder
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

	// Get the localized start message
	startMsg := locales.GetMessage(localizer, "MsgStart", nil, nil)

	return h.sendSuccess(ctx, bot, message.Chat.ID, startMsg)
}

// HandleHelp handles the /help command.
// It generates a help message listing available commands, updates user info, logs the action, and sends the help text.
func (h *MessageHandler) HandleHelp(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID

	// Create localizer
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode // TODO: Detect user language
	}
	localizer := locales.NewLocalizer(lang)

	// Check if the user is an admin
	isAdmin := false // Default to false
	if h.suggestionManager != nil {
		var checkErr error
		isAdmin, checkErr = h.suggestionManager.IsAdmin(ctx, userID)
		if checkErr != nil {
			log.Printf("Error checking admin status for user %d: %v. Assuming non-admin.", userID, checkErr)
			isAdmin = false // Treat error as non-admin for safety
		}
		log.Printf("User %d admin status for /help: %t", userID, isAdmin)
	} else {
		log.Printf("Warning: Suggestion manager is nil, cannot check admin status for user %d", userID)
	}

	var helpText strings.Builder
	helpText.WriteString(locales.GetMessage(localizer, "MsgHelpHeader", nil, nil) + "\n") // Add a header key

	// Filter commands based on admin status
	for _, cmd := range h.commands {
		showCommand := false
		if isAdmin {
			// Admins see all commands except /suggest
			if cmd.Command != "suggest" {
				showCommand = true
			}
		} else {
			// Non-admins see only /start and /suggest
			if cmd.Command == "start" || cmd.Command == "suggest" {
				showCommand = true
			}
		}

		if showCommand {
			// Localize command description
			// Use the Description field directly as it now holds the key
			localizedDesc := locales.GetMessage(localizer, cmd.Description, nil, nil)
			helpText.WriteString(fmt.Sprintf("/%s - %s\n", cmd.Command, localizedDesc))
		}
	}
	// Select and localize the appropriate footer
	var footerKey string
	if isAdmin {
		footerKey = "MsgHelpFooterAdmin"
	} else {
		footerKey = "MsgHelpFooterUser"
	}
	helpText.WriteString(locales.GetMessage(localizer, footerKey, nil, nil))

	// Update user info (record admin status)
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_help")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /help: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log help command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_help", map[string]interface{}{
		"chat_id":  message.Chat.ID,
		"is_admin": isAdmin, // Log admin status
	})
	if err != nil {
		log.Printf("Failed to log /help command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, helpText.String())
}

// HandleStatus handles the /status command.
// It retrieves the current active caption, formats a status message, updates user info, logs the action, and sends the status.
func (h *MessageHandler) HandleStatus(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID)

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Get localized status message
	statusText := locales.GetMessage(localizer, "MsgStatus", map[string]interface{}{
		"ChannelID": h.channelID,
		"Caption":   caption,
	}, nil)

	// Placeholder: Determine admin status
	isAdmin := false
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_status")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /status: %v", message.From.ID, err)
	}

	// Log status command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_status", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"caption": caption,
	})
	if err != nil {
		log.Printf("Failed to log /status command for user %d: %v", message.From.ID, err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command.
// It retrieves the application version, formats a version message, updates user info, logs the action, and sends the version.
func (h *MessageHandler) HandleVersion(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Get localized version message
	versionText := locales.GetMessage(localizer, "MsgVersion", map[string]interface{}{
		"Version": version,
	}, nil)

	// Placeholder: Determine admin status
	isAdmin := false
	err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "command_version")
	if err != nil {
		log.Printf("Failed to update user info for user %d during /version: %v", message.From.ID, err)
	}

	// Log version command action
	err = h.actionLogger.LogUserAction(message.From.ID, "command_version", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"version": version,
	})
	if err != nil {
		log.Printf("Failed to log /version command for user %d: %v", message.From.ID, err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, versionText)
}

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
// It builds the list of commands from the handler's configuration, localizes their descriptions,
// and uses the bot instance to set them.
func (h *MessageHandler) setupCommands(ctx context.Context, bot *telego.Bot) error {
	if len(h.commands) == 0 {
		log.Println("No commands defined in handler, skipping SetMyCommands.")
		return nil // No commands to set is not an error
	}

	// Create a localizer for the default language to translate descriptions
	localizer := locales.NewLocalizer(locales.DefaultLanguage)

	commands := make([]telego.BotCommand, 0, len(h.commands)) // Initialize with capacity
	for _, cmd := range h.commands {
		// Get the localized description using the key stored in cmd.Description
		localizedDesc := locales.GetMessage(localizer, cmd.Description, nil, nil)
		commands = append(commands, telego.BotCommand{
			Command:     cmd.Command,
			Description: localizedDesc, // Use the translated description
		})
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
