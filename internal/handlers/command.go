package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"vrcmemes-bot/internal/locales"
	telegoapi "vrcmemes-bot/pkg/telegoapi" // Import for BotAPI

	// Add import for bot package
	// "time" // time is not used directly in this file after logger refactoring

	"github.com/mymmrac/telego"
)

// HandleStart handles the /start command.
// It sets up the bot commands, updates user info, logs the action, and sends a welcome message.
func (h *MessageHandler) HandleStart(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
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
	h.RecordUserActivity(ctx, message.From, ActionCommandStart, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
	})

	// Get the localized start message
	startMsg := locales.GetMessage(localizer, "MsgStart", nil, nil)

	return h.sendSuccess(ctx, bot, message.Chat.ID, startMsg)
}

// HandleHelp handles the /help command.
// It generates a help message listing available commands, updates user info, logs the action, and sends the help text.
func (h *MessageHandler) HandleHelp(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
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
	h.RecordUserActivity(ctx, message.From, ActionCommandHelp, isAdmin, map[string]interface{}{
		"chat_id":  message.Chat.ID,
		"is_admin": isAdmin, // Log admin status used for help message
	})

	return h.sendSuccess(ctx, bot, message.Chat.ID, helpText.String())
}

// HandleStatus handles the /status command.
// It retrieves the current active caption, formats a status message, updates user info, logs the action, and sends the status.
func (h *MessageHandler) HandleStatus(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
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
	h.RecordUserActivity(ctx, message.From, ActionCommandStatus, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
		"caption": caption, // Log the caption that was active
	})

	return h.sendSuccess(ctx, bot, message.Chat.ID, statusText)
}

// HandleVersion handles the /version command.
// It retrieves the application version, formats a version message, updates user info, logs the action, and sends the version.
func (h *MessageHandler) HandleVersion(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
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
	h.RecordUserActivity(ctx, message.From, ActionCommandVersion, isAdmin, map[string]interface{}{
		"chat_id": message.Chat.ID,
		"version": version,
	})

	return h.sendSuccess(ctx, bot, message.Chat.ID, versionText)
}

// HandleSuggest handles the /suggest command by calling the suggestion manager.
// It constructs a telego.Update object and passes it to the suggestion manager's handler.
// Errors during suggestion handling are logged, assuming the manager handles user feedback.
func (h *MessageHandler) HandleSuggest(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
	userID := message.From.ID
	// Construct update for manager and delegate
	update := telego.Update{Message: &message}
	if h.suggestionManager != nil {
		err := h.suggestionManager.HandleSuggestCommand(ctx, update)
		if err != nil {
			// Log error from HandleSuggestCommand. It should handle user feedback itself.
			log.Printf("[Cmd:suggest User:%d] Error from suggestionManager.HandleSuggestCommand: %v", userID, err)
			// Return nil because HandleSuggestCommand should manage sending errors to the user.
			return nil
		}
		return nil // Success
	} else {
		log.Printf("[Cmd:suggest User:%d] Error: Suggestion manager is nil?", userID)
		localizer := h.getLocalizer(message.From) // Use helper
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
	}
}

// HandleReview handles the /review command by delegating to the suggestion manager.
func (h *MessageHandler) HandleReview(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
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

// HandleFeedback simply delegates to the suggestion manager's HandleFeedbackCommand.
func (h *MessageHandler) HandleFeedback(ctx context.Context, bot telegoapi.BotAPI, message telego.Message) error {
	userID := message.From.ID
	update := telego.Update{Message: &message}
	if h.suggestionManager != nil {
		err := h.suggestionManager.HandleFeedbackCommand(ctx, update)
		if err != nil {
			// Log error from HandleFeedbackCommand. It should handle user feedback itself.
			log.Printf("[Cmd:feedback User:%d] Error from suggestionManager.HandleFeedbackCommand: %v", userID, err)
			// Return nil because HandleFeedbackCommand should manage sending errors to the user.
			return nil
		}
		return nil // Success
	} else {
		log.Printf("[Cmd:feedback User:%d] Error: Suggestion manager is nil?", userID)
		localizer := h.getLocalizer(message.From) // Use helper
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
	}
}

// --- Helper Functions ---

// setupCommands registers the bot's commands with Telegram.
// It builds the list of commands from the handler's configuration, localizes their descriptions,
// and uses the bot instance to set them.
func (h *MessageHandler) setupCommands(ctx context.Context, bot telegoapi.BotAPI) error {
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
