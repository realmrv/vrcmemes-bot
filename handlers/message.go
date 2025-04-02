package handlers

import (
	"context"
	"fmt"
	"log"
	"time"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
	tu "github.com/mymmrac/telego/telegoutil"
)

// HandleText handles incoming text messages.
// If the user is in the process of setting a caption, it captures the text as the caption.
// Otherwise, if the user is an admin, it forwards the text message to the configured channel.
// It logs the action and updates user information.
func (h *MessageHandler) HandleText(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// Ignore empty messages or the /start command itself (handled by command handler)
	if message.Text == "" || message.Text == "/start" {
		return nil
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Check if we are waiting for a caption from this user/chat
	if _, waiting := h.waitingForCaption.Load(message.Chat.ID); waiting {
		_, hadPreviousCaption := h.GetActiveCaption(message.Chat.ID)
		h.setActiveCaption(message.Chat.ID, message.Text)
		h.waitingForCaption.Delete(message.Chat.ID) // Done waiting

		// Update user information (Placeholder for isAdmin check)
		isAdmin := false // Placeholder
		err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "set_caption_via_text")
		if err != nil {
			log.Printf("Failed to update user info after setting caption for user %d: %v", message.From.ID, err)
			// Potentially send to Sentry
		}

		// Log caption setting action
		err = h.actionLogger.LogUserAction(message.From.ID, "set_caption_via_text", map[string]interface{}{
			"chat_id":   message.Chat.ID,
			"caption":   message.Text,
			"overwrite": hadPreviousCaption,
		})
		if err != nil {
			log.Printf("Failed to log caption set action for user %d: %v", message.From.ID, err)
			// Potentially send to Sentry
		}

		// Send confirmation message using localized strings
		var confirmationMsgID string
		if hadPreviousCaption {
			confirmationMsgID = "MsgCaptionOverwriteConfirmation"
		} else {
			confirmationMsgID = "MsgCaptionSetConfirmation"
		}
		msg := locales.GetMessage(localizer, confirmationMsgID, nil, nil)
		return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
	}

	// --- Handling regular text message (forwarding to channel if admin) ---

	// Check admin rights before posting text to channel
	isAdmin, err := h.isUserAdmin(ctx, bot, message.From.ID)
	if err != nil {
		// Use localized error message?
		wrappedErr := fmt.Errorf("failed to check admin status: %w", err)
		// errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil) // Example
		return h.sendError(ctx, bot, message.Chat.ID, wrappedErr) // Keep original error for now
	}
	if !isAdmin {
		// Inform the user using localized message
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
	}

	// User is admin, proceed to send the message to the channel
	sentMsg, err := bot.SendMessage(ctx, tu.Message(
		tu.ID(h.channelID),
		message.Text,
	).WithParseMode(telego.ModeHTML),
	)
	if err != nil {
		// Wrap error from SendMessage
		return h.sendError(ctx, bot, message.Chat.ID, fmt.Errorf("failed to send text message to channel %d: %w", h.channelID, err))
	}

	publishedTime := time.Now()

	// Create log entry for the text message post
	logEntry := models.PostLog{
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        message.Text, // For text messages, caption is the text itself
		MessageType:    "text",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsg.MessageID,
	}

	// Log the post to the database
	if err := h.postLogger.LogPublishedPost(logEntry); err != nil {
		// LogPublishedPost already logs internally, just add user ID context here
		log.Printf("Failed attempt to log text post to DB from user %d. Error: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Update user information
	err = h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_text_to_channel")
	if err != nil {
		log.Printf("Failed to update user info after sending text for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log text message sending action
	err = h.actionLogger.LogUserAction(message.From.ID, "send_text_to_channel", map[string]interface{}{
		"chat_id":            message.Chat.ID,
		"text":               message.Text,
		"channel_message_id": sentMsg.MessageID,
	})
	if err != nil {
		log.Printf("Failed to log send_text_to_channel action for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Send confirmation using localized message
	msg := locales.GetMessage(localizer, "MsgPostSentToChannel", nil, nil)
	return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
}

// HandlePhoto handles incoming photo messages.
// If the user is an admin, it copies the photo message to the configured channel, applying the active caption if set.
// It logs the action and updates user information.
func (h *MessageHandler) HandlePhoto(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// Ignore messages that are not photos (e.g., text messages with photo URLs)
	if message.Photo == nil {
		log.Printf("HandlePhoto called with non-photo message (ID: %d) from user %d", message.MessageID, message.From.ID)
		return nil // Or handle as an error? For now, just ignore.
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Check admin rights before posting photo to channel
	isAdmin, err := h.isUserAdmin(ctx, bot, message.From.ID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to check admin status: %w", err)
		return h.sendError(ctx, bot, message.Chat.ID, wrappedErr) // Keep original error
	}
	if !isAdmin {
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
	}

	// Get the currently active caption for this user/chat (if any)
	caption, _ := h.GetActiveCaption(message.Chat.ID) // Assuming GetActiveCaption returns empty string if none

	// Copy the photo message to the target channel
	sentMsgID, err := bot.CopyMessage(ctx, &telego.CopyMessageParams{
		ChatID:     tu.ID(h.channelID),
		FromChatID: tu.ID(message.Chat.ID),
		MessageID:  message.MessageID,
		Caption:    caption, // Apply the active caption
	})
	if err != nil {
		// Wrap error from CopyMessage
		return h.sendError(ctx, bot, message.Chat.ID, fmt.Errorf("failed to copy photo message %d to channel %d: %w", message.MessageID, h.channelID, err))
	}

	publishedTime := time.Now()

	// Create log entry for the photo post
	logEntry := models.PostLog{
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        caption,
		MessageType:    "photo",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		// Use the MessageID from the result of CopyMessage which is the ID in the destination channel
		ChannelPostID: sentMsgID.MessageID,
	}

	// Log the post to the database
	if err := h.postLogger.LogPublishedPost(logEntry); err != nil {
		log.Printf("Failed attempt to log photo post to DB from user %d. Error: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Update user information
	err = h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_photo_to_channel")
	if err != nil {
		log.Printf("Failed to update user info after sending photo for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Log photo sending action
	err = h.actionLogger.LogUserAction(message.From.ID, "send_photo_to_channel", map[string]interface{}{
		"chat_id":             message.Chat.ID,
		"original_message_id": message.MessageID,
		"channel_message_id":  sentMsgID.MessageID,
		"caption_used":        caption,
	})
	if err != nil {
		log.Printf("Failed to log send_photo_to_channel action for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Send confirmation using localized message
	msg := locales.GetMessage(localizer, "MsgPostSentToChannel", nil, nil)
	return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
}

// ProcessSuggestionMessage checks if the message is part of the suggestion workflow
// and delegates handling to the suggestion manager if appropriate.
// It returns true if the message was handled by the suggestion manager, false otherwise.
func (h *MessageHandler) ProcessSuggestionMessage(ctx context.Context, update telego.Update) (bool, error) {
	if h.suggestionManager == nil {
		// This indicates a setup issue if the manager is expected
		log.Println("WARN: ProcessSuggestionMessage called but suggestionManager is nil")
		return false, nil
	}
	// Delegate to the suggestion manager's message handler
	processed, err := h.suggestionManager.HandleMessage(ctx, update)
	if err != nil {
		// Log errors from the suggestion manager's handling
		log.Printf("Error from suggestionManager.HandleMessage: %v", err)
		// Return the error so the main loop can potentially handle it (e.g., Sentry)
		return processed, fmt.Errorf("suggestion manager failed to handle message: %w", err)
	}
	return processed, nil
}

// HandleMediaGroup is commented out as media group logic is likely handled in bot.go
/*
func (h *MessageHandler) HandleMediaGroup(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// ... (Potential future implementation or removal) ...
}
*/

// sendError sends a standardized, localized error message to the user.
func (h *MessageHandler) sendError(ctx context.Context, bot *telego.Bot, chatID int64, err error) error {
	lang := locales.DefaultLanguage
	// TODO: Determine language based on chatID preferences if possible
	localizer := locales.NewLocalizer(lang)

	// Log the original error for debugging
	log.Printf("Sending error message for chat %d: %v", chatID, err)

	// Send localized message to the user, potentially using the error message
	// Note: We are using MsgErrorGeneral here, which doesn't have an {{.Error}} template.
	// If you want to include the specific error, consider a different message ID or modifying MsgErrorGeneral.
	msg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil) // Pass nil for data as MsgErrorGeneral doesn't use it

	_, sendErr := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
	if sendErr != nil {
		log.Printf("Failed to send error message to chat %d: %v", chatID, sendErr)
	}
	return err // Return the original error
}
