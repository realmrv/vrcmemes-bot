package handlers

import (
	"context"
	"log"
	"vrcmemes-bot/internal/locales"
	telegoapi "vrcmemes-bot/pkg/telegoapi" // Import for BotAPI

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// sendSuccess sends a generic success message to the user.
func (h *MessageHandler) sendSuccess(ctx context.Context, bot telegoapi.BotAPI, chatID int64, text string) error { // Use telegoapi.BotAPI
	_, err := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	if err != nil {
		log.Printf("Error sending success message to chat %d: %v", chatID, err)
		// Don't return error to user, just log it.
	}
	return nil
}

// sendError sends a generic error message to the user.
// Logs the original error.
func (h *MessageHandler) sendError(ctx context.Context, bot telegoapi.BotAPI, chatID int64, originalErr error) error { // Use telegoapi.BotAPI
	// Log the original error for debugging
	log.Printf("Error for user in chat %d: %v", chatID, originalErr)

	// Attempt to send a generic, localized error message to the user
	// TODO: Determine user language correctly if possible
	localizer := locales.NewLocalizer(locales.DefaultLanguage)
	errMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)

	_, sendErr := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errMsg))
	if sendErr != nil {
		log.Printf("Error sending generic error message to chat %d: %v", chatID, sendErr)
	}

	// Return the original error to allow the main loop to handle it (e.g., Sentry logging)
	return originalErr
}

// getLocalizer determines the best localizer for a given user.
// Currently, it defaults to Russian if the user object is nil or language code is empty/unsupported.
func (h *MessageHandler) getLocalizer(user *telego.User) *i18n.Localizer {
	lang := locales.DefaultLanguage
	if user != nil && user.LanguageCode != "" {
		// Check if the language is supported by looking at loaded tags
		// We need to access the bundle directly for this, which is not ideal from here.
		// Let's assume NewLocalizer handles fallback gracefully for now.
		// TODO: Refactor language checking if needed
		lang = user.LanguageCode // Pass the user's code to NewLocalizer
	}
	return locales.NewLocalizer(lang)
}

// recordUserActivity combines updating user info and logging the action.
func (h *MessageHandler) RecordUserActivity(ctx context.Context, user *telego.User, action string, isAdmin bool, details map[string]interface{}) {
	if user == nil {
		log.Printf("Attempted to record activity for nil user, action: %s", action)
		return
	}

	// Update user info in the database
	if err := h.userRepo.UpdateUser(ctx, user.ID, user.Username, user.FirstName, user.LastName, isAdmin, action); err != nil {
		log.Printf("Error updating user %d (%s) in DB during action %s: %v", user.ID, user.Username, action, err)
		// Continue to log the action even if DB update fails
	}

	// Log the user action
	if err := h.actionLogger.LogUserAction(user.ID, action, details); err != nil {
		log.Printf("Error logging action %s for user %d (%s): %v", action, user.ID, user.Username, err)
	}
}

// GetActiveCaption retrieves the currently stored active caption for a chat.
func (h *MessageHandler) GetActiveCaption(chatID int64) (string, bool) {
	if caption, ok := h.activeCaptions.Load(chatID); ok {
		if capStr, typeOk := caption.(string); typeOk {
			return capStr, true
		}
	}
	return "", false
}

// HandleCallbackQuery processes callback queries, primarily delegating to the suggestion manager.
// It also sends an acknowledgment to Telegram to stop the loading indicator on the button.
func (h *MessageHandler) HandleCallbackQuery(ctx context.Context, bot telegoapi.BotAPI, query telego.CallbackQuery) error {
	processed := false
	var processingErr error

	// Acknowledge the callback query immediately to stop the loading spinner
	ackParams := &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID}
	if err := bot.AnswerCallbackQuery(ctx, ackParams); err != nil {
		// Log the error but don't necessarily stop processing if possible
		log.Printf("Error answering callback query %s: %v", query.ID, err)
	}

	// Delegate to suggestion manager if available
	if h.suggestionManager != nil {
		processed, processingErr = h.suggestionManager.HandleCallbackQuery(ctx, query)
		if processingErr != nil {
			// Log the error from the suggestion manager
			log.Printf("Error processing callback query %s via suggestion manager: %v", query.ID, processingErr)
			// Optionally send a generic error message back via AnswerCallbackQuery with text?
			// Example: bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "An error occurred"})
			// For now, just log and return the error.
			return processingErr // Return the error from the manager
		}
	}

	if !processed {
		// If no manager handled it, log this situation.
		log.Printf("Callback query %s not processed by any manager. Data: %s", query.ID, query.Data)
		// Optionally send a generic message like "Action not supported"?
		// For now, just return nil as we acknowledged the query.
	}

	return nil // Return nil if processed or handled gracefully
}
