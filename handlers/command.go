package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	// "time" // time is not used directly in this file after logger refactoring

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // th is no longer needed
	// Assuming config.Version is needed
	"vrcmemes-bot/pkg/locales" // Import locales package

	"github.com/nicksnyder/go-i18n/v2/i18n" // Correct import for i18n.Localizer
)

// HandleStart handles the /start command.
// It sets up the bot commands, updates user info, logs the action, and sends a welcome message.
func (h *MessageHandler) HandleStart(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	if err := h.setupCommands(ctx, bot); err != nil {
		// Consider adding more context to the error before sending
		// Maybe use locales for this error message too?
		return h.sendError(ctx, bot, message.Chat.ID, fmt.Errorf("failed to set up commands: %w", err))
	}

	localizer := h.getLocalizer(message.From) // Use helper

	// Placeholder: Determine if the user is an admin (requires implementation)
	// Let's assume checkAdmin is the way to go, even if it's false for start now.
	isAdmin, _ := h.checkAdmin(ctx, message.From.ID) // Use helper, ignore error for now

	// Record activity (UpdateUser + LogUserAction combined)
	h.recordUserActivity(ctx, message.From, ActionCommandStart, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
	})

	// Get the localized start message
	startMsg := locales.GetMessage(localizer, "MsgStart", nil, nil)

	return h.sendSuccess(ctx, bot, message.Chat.ID, startMsg)
}

// HandleHelp handles the /help command.
// It generates a help message listing available commands, updates user info, logs the action, and sends the help text.
func (h *MessageHandler) HandleHelp(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID
	localizer := h.getLocalizer(message.From) // Use helper

	// Check if the user is an admin
	isAdmin, _ := h.checkAdmin(ctx, userID) // Use helper, ignore error as checkAdmin logs it
	// Log admin status check result for debugging /help specifically
	log.Printf("[Cmd:help User:%d] Admin status check result: %t", userID, isAdmin)

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

	// Record activity (UpdateUser + LogUserAction combined)
	h.recordUserActivity(ctx, message.From, ActionCommandHelp, isAdmin, map[string]interface{}{
		"chat_id":  message.Chat.ID,
		"is_admin": isAdmin, // Log admin status used for help message
	})

	return h.sendSuccess(ctx, bot, message.Chat.ID, helpText.String())
}

// HandleStatus handles the /status command.
// It retrieves the current active caption, formats a status message, updates user info, logs the action, and sends the status.
func (h *MessageHandler) HandleStatus(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, _ := h.GetActiveCaption(message.Chat.ID)
	localizer := h.getLocalizer(message.From) // Use helper

	// Get localized status message
	statusText := locales.GetMessage(localizer, "MsgStatus", map[string]interface{}{
		"ChannelID": h.channelID,
		"Caption":   caption,
	}, nil)

	// Check admin status (even if not used by logic, good to record)
	isAdmin, _ := h.checkAdmin(ctx, message.From.ID)

	// Record activity
	h.recordUserActivity(ctx, message.From, ActionCommandStatus, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
		"caption": caption, // Log the caption that was active
	})

	return h.sendSuccess(ctx, bot, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command.
// It retrieves the application version, formats a version message, updates user info, logs the action, and sends the version.
func (h *MessageHandler) HandleVersion(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}

	localizer := h.getLocalizer(message.From) // Use helper

	// Get localized version message
	versionText := locales.GetMessage(localizer, "MsgVersion", map[string]interface{}{
		"Version": version,
	}, nil)

	// Check admin status (even if not used by logic, good to record)
	isAdmin, _ := h.checkAdmin(ctx, message.From.ID)

	// Record activity
	h.recordUserActivity(ctx, message.From, ActionCommandVersion, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
		"version": version,
	})

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

// HandleReview handles the /review command by delegating to the suggestion manager.
func (h *MessageHandler) HandleReview(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID

	// --- Admin Check ---
	isAdmin, err := h.checkAdmin(ctx, userID) // Use helper
	if err != nil {
		// checkAdmin already logged the error. We might want to report to Sentry here.
		// sentry.CaptureException(fmt.Errorf("admin check failed for /review user %d: %w", userID, err))
		// Decide if we need to send an error message if the check itself failed (e.g., manager nil)
		if errors.Is(err, errors.New("suggestion manager not initialized")) { // Example check
			localizer := h.getLocalizer(message.From)
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg)) // Return the generic error message
		}
		// If it was just a DB error or similar during IsAdmin, maybe we don't send error to user?
		// For now, let's assume non-admin if check failed for reasons other than nil manager.
		isAdmin = false
	}

	if !isAdmin {
		// Log attempt and send specific error message
		log.Printf("[Cmd:review User:%d] Non-admin user attempted to use /review.", userID)
		localizer := h.getLocalizer(message.From) // Use helper
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		// Don't record activity for failed attempts?
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(msg))
	}
	// --- End Admin Check ---

	// User is admin, record activity (optional for review start?)
	// h.recordUserActivity(ctx, message.From, ActionCommandReview, isAdmin, map[string]interface{}{"chat_id": message.Chat.ID})

	// Construct update for manager and delegate
	update := telego.Update{Message: &message}
	if h.suggestionManager != nil {
		err = h.suggestionManager.HandleReviewCommand(ctx, update)
		if err != nil {
			// Log error from HandleReviewCommand. It should handle user feedback itself.
			log.Printf("[Cmd:review User:%d] Error from suggestionManager.HandleReviewCommand: %v", userID, err)
			// Return nil because HandleReviewCommand should manage sending errors to the user.
			return nil
		}
		return nil // Success
	} else {
		// This case should technically be caught by checkAdmin, but added for safety
		log.Printf("[Cmd:review User:%d] Error: Suggestion manager became nil after admin check?", userID)
		localizer := h.getLocalizer(message.From) // Use helper
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
	}
}

// --- Helper Functions ---

// checkAdmin checks if the user is an admin using the suggestion manager.
// It returns the admin status and any error encountered during the check.
// It logs warnings/errors internally.
func (h *MessageHandler) checkAdmin(ctx context.Context, userID int64) (isAdmin bool, err error) {
	if h.suggestionManager == nil {
		log.Printf("[AdminCheck User:%d] Warning: Suggestion manager is nil", userID)
		// Return false and an error to indicate the check couldn't be performed reliably.
		return false, errors.New("suggestion manager not initialized")
	}
	isAdmin, err = h.suggestionManager.IsAdmin(ctx, userID)
	if err != nil {
		log.Printf("[AdminCheck User:%d] Error checking admin status: %v. Assuming non-admin.", userID, err)
		// Return false and the original error for potential upstream logging (e.g., Sentry)
		return false, err
	}
	return isAdmin, nil
}

// getLocalizer creates a localizer instance based on the user's language preference.
// Falls back to the default language if the user or their language code is unavailable.
func (h *MessageHandler) getLocalizer(user *telego.User) *i18n.Localizer {
	lang := locales.DefaultLanguage
	if user != nil && user.LanguageCode != "" {
		// TODO: Implement robust language selection based on supported languages
		// Example: if locales.IsSupported(user.LanguageCode) { lang = user.LanguageCode }
		// log.Printf("[Localizer User:%d] Using language: %s", user.ID, lang) // Optional debug log
	}
	return locales.NewLocalizer(lang)
}

// recordUserActivity updates user info in the repository and logs the user action.
// It logs errors internally if updates or logging fail.
func (h *MessageHandler) recordUserActivity(ctx context.Context, user *telego.User, action string, isAdmin bool, details map[string]interface{}) {
	if user == nil {
		log.Printf("[Activity] Cannot record activity for action '%s': user is nil", action)
		return
	}

	// Update user information
	if h.userRepo != nil {
		err := h.userRepo.UpdateUser(ctx, user.ID, user.Username, user.FirstName, user.LastName, isAdmin, action)
		if err != nil {
			log.Printf("[Activity User:%d Action:%s] Failed to update user info: %v", user.ID, action, err)
			// Consider sending to Sentry: sentry.CaptureException(fmt.Errorf("failed to update user %d for action %s: %w", user.ID, action, err))
		}
	} else {
		log.Printf("[Activity User:%d Action:%s] Warning: UserRepository is nil, cannot update user info", user.ID, action)
	}

	// Log the user action
	if h.actionLogger != nil {
		err := h.actionLogger.LogUserAction(user.ID, action, details)
		if err != nil {
			log.Printf("[Activity User:%d Action:%s] Failed to log user action: %v", user.ID, action, err)
			// Consider sending to Sentry: sentry.CaptureException(fmt.Errorf("failed to log action %s for user %d: %w", user.ID, action, err))
		}
	} else {
		log.Printf("[Activity User:%d Action:%s] Warning: UserActionLogger is nil, cannot log action", user.ID, action)
	}
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
