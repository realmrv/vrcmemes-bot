package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/internal/suggestions"

	// Add import for bot package
	// "time" // time is not used directly in this file after logger refactoring

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

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
	isAdmin, _ := h.adminChecker.IsAdmin(ctx, message.From.ID) // Use checker method

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
	isAdmin, _ := h.adminChecker.IsAdmin(ctx, userID) // Use checker method
	// Log admin status check result for debugging /help specifically
	log.Printf("[Cmd:help User:%d] Admin status check result: %t", userID, isAdmin)

	var helpText strings.Builder
	helpText.WriteString(locales.GetMessage(localizer, "MsgHelpHeader", nil, nil) + "\n") // Add a header key

	// Filter commands based on admin status
	for _, cmd := range h.commands {
		showCommand := false
		if isAdmin {
			// Admins see all commands except /suggest and /feedback
			if cmd.Command != "suggest" && cmd.Command != "feedback" {
				showCommand = true
			}
		} else {
			// Non-admins see only /start, /suggest, and /feedback
			if cmd.Command == "start" || cmd.Command == "suggest" || cmd.Command == "feedback" {
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
	isAdmin, _ := h.adminChecker.IsAdmin(ctx, message.From.ID) // Use checker method

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
	isAdmin, _ := h.adminChecker.IsAdmin(ctx, message.From.ID) // Use checker method

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
	userID := message.From.ID
	chatID := message.Chat.ID
	localizer := h.getLocalizer(message.From)

	// --- Admin Check: Prevent admins from using /suggest ---
	isAdmin, err := h.adminChecker.IsAdmin(ctx, userID)
	if err != nil {
		// Log error but proceed cautiously (treat as non-admin for check)
		log.Printf("[Cmd:suggest User:%d] Error checking admin status: %v. Allowing command for now.", userID, err)
		isAdmin = false // Assume non-admin if check fails, maybe allow suggest?
	}
	if isAdmin {
		log.Printf("[Cmd:suggest User:%d] Admin attempted to use /suggest.", userID)
		msg := locales.GetMessage(localizer, "MsgSuggestForUsersOnly", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg)) // Use sendError helper
	}
	// --- End Admin Check ---

	// Check if already waiting for suggestion
	if h.suggestionManager.GetUserState(userID) == suggestions.StateAwaitingSuggestion {
		msg := locales.GetMessage(localizer, "MsgSuggestAlreadyWaitingForContent", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}

	// We need the full Update object for the manager's handler
	// Construct a minimal Update containing the Message
	update := telego.Update{Message: &message}

	// Delegate the handling to the suggestion manager
	err = h.suggestionManager.HandleSuggestCommand(ctx, update)
	if err != nil {
		// The manager is expected to handle sending messages to the user on errors
		// (e.g., user not subscribed, invalid format). We just log the error here
		// if one occurs during the manager's processing.
		log.Printf("[HandleSuggest] Error from suggestionManager.HandleSuggestCommand for user %d: %v", userID, err)
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
	isAdmin, err := h.adminChecker.IsAdmin(ctx, userID) // Use checker method
	if err != nil {
		// sentry.CaptureException(fmt.Errorf("admin check failed for /review user %d: %w", userID, err))
		if errors.Is(err, errors.New("failed to get chat member info")) { // Check for the specific error from IsAdmin
			log.Printf("[Cmd:review User:%d] Error checking admin status: %v", userID, err)
			localizer := h.getLocalizer(message.From)
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
		}
		log.Printf("[Cmd:review User:%d] Unexpected error during admin check: %v. Assuming non-admin.", userID, err)
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

// HandleFeedback handles the /feedback command.
// It initiates the feedback process by setting the user state to StateAwaitingFeedback
// and prompting the user to send their feedback message.
func (h *MessageHandler) HandleFeedback(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID
	chatID := message.Chat.ID
	localizer := h.getLocalizer(message.From)

	// --- Admin Check: Prevent admins from using /feedback ---
	isAdmin, err := h.adminChecker.IsAdmin(ctx, userID)
	if err != nil {
		// Log error but proceed cautiously (treat as non-admin for check)
		log.Printf("[Cmd:feedback User:%d] Error checking admin status: %v. Allowing command for now.", userID, err)
		isAdmin = false // Assume non-admin if check fails
	}
	if isAdmin {
		log.Printf("[Cmd:feedback User:%d] Admin attempted to use /feedback.", userID)
		msg := locales.GetMessage(localizer, "MsgFeedbackForUsersOnly", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}
	// --- End Admin Check ---

	// Check if already waiting for feedback
	if h.suggestionManager.GetUserState(userID) == suggestions.StateAwaitingFeedback {
		msg := locales.GetMessage(localizer, "MsgFeedbackAlreadyWaiting", nil, nil)
		_, err = bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return err
	}

	// Set user state to awaiting feedback
	h.suggestionManager.SetUserState(userID, suggestions.StateAwaitingFeedback)

	// Send prompt message
	promptMsg := locales.GetMessage(localizer, "MsgFeedbackPrompt", nil, nil)
	_, err = bot.SendMessage(ctx, tu.Message(tu.ID(chatID), promptMsg))
	if err != nil {
		// Rollback state if sending prompt fails
		h.suggestionManager.SetUserState(userID, suggestions.StateIdle)
		log.Printf("Error sending feedback prompt to user %d: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		return err
	}

	// Record activity
	h.recordUserActivity(ctx, message.From, ActionCommandFeedback, false, map[string]interface{}{
		"chat_id": chatID,
	})

	return nil
}

// --- Helper Functions ---

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
