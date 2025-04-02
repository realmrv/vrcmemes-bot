package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
	tu "github.com/mymmrac/telego/telegoutil"
)

// HandleText handles incoming text messages (excluding commands).
// It checks if the user is an admin and then processes the text for publishing or setting as caption.
func (h *MessageHandler) HandleText(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID
	chatID := message.Chat.ID

	// Check if we are waiting for a caption from this chat
	if _, waiting := h.waitingForCaption.Load(chatID); waiting {
		captionText := message.Text
		log.Printf("Received caption text \"%s\" from user %d (Chat %d)", captionText, userID, chatID)

		// Store the caption
		_, exists := h.activeCaptions.Load(chatID)
		h.activeCaptions.Store(chatID, captionText)
		h.waitingForCaption.Delete(chatID) // Clear the waiting state

		// Send confirmation
		lang := locales.DefaultLanguage
		if message.From != nil && message.From.LanguageCode != "" {
			// lang = message.From.LanguageCode
		}
		localizer := locales.NewLocalizer(lang)
		var confirmationMsg string
		if exists {
			confirmationMsg = locales.GetMessage(localizer, "MsgCaptionOverwriteConfirmation", nil, nil)
		} else {
			confirmationMsg = locales.GetMessage(localizer, "MsgCaptionSetConfirmation", nil, nil)
		}

		// Log action
		err := h.actionLogger.LogUserAction(userID, "set_caption_reply", map[string]interface{}{
			"chat_id": chatID,
			"caption": captionText,
		})
		if err != nil {
			log.Printf("Failed to log set_caption_reply action for user %d: %v", userID, err)
		}
		return h.sendSuccess(ctx, bot, chatID, confirmationMsg)
	}

	// If not waiting for caption, proceed with admin check and potential direct posting
	// --- Admin Check ---
	isAdmin := false
	if h.suggestionManager != nil {
		var checkErr error
		isAdmin, checkErr = h.suggestionManager.IsAdmin(ctx, userID)
		if checkErr != nil {
			log.Printf("Error checking admin status for user %d in HandleText: %v. Assuming non-admin.", userID, checkErr)
			isAdmin = false
		}
	} else {
		log.Printf("Warning: Suggestion manager is nil in HandleText, cannot check admin status for user %d", userID)
	}

	if !isAdmin {
		log.Printf("User %d (not admin) attempted to send text directly.", userID)
		// Non-admins cannot send text directly for publishing
		// Should we send an error? Or just ignore? Sending error for now.
		lang := locales.DefaultLanguage
		if message.From != nil && message.From.LanguageCode != "" {
			// lang = message.From.LanguageCode
		}
		localizer := locales.NewLocalizer(lang)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}
	// --- End Admin Check ---

	// Admin is sending text directly for publishing
	log.Printf("Admin %d sending text message to channel %d", userID, h.channelID)
	textToPublish := message.Text

	// TODO: Consider if admins should be able to set caption with simple text? Unlikely.

	sentMsg, err := bot.SendMessage(ctx, tu.Message(tu.ID(h.channelID), textToPublish))
	if err != nil {
		return h.sendError(ctx, bot, chatID, fmt.Errorf("failed to send text message to channel %d: %w", h.channelID, err))
	}

	// Log the successful post
	log.Printf("Admin %d successfully sent text message %d to channel %d", userID, sentMsg.MessageID, h.channelID)

	// Create log entry for the text message post
	logEntry := models.PostLog{
		SenderID:       userID,
		SenderUsername: message.From.Username,
		Caption:        message.Text, // For text messages, caption is the text itself
		MessageType:    "text",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    time.Unix(int64(sentMsg.Date), 0),
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsg.MessageID,
	}
	if err := h.postLogger.LogPublishedPost(logEntry); err != nil {
		log.Printf("Failed attempt to log text post to DB from user %d. Error: %v", userID, err)
	}

	// Update user information
	err = h.userRepo.UpdateUser(ctx, userID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_text_to_channel")
	if err != nil {
		log.Printf("Failed to update user info after sending text for user %d: %v", userID, err)
	}

	// Log text message sending action
	err = h.actionLogger.LogUserAction(userID, "send_text_to_channel", map[string]interface{}{
		"chat_id":            chatID,
		"text":               message.Text,
		"channel_message_id": sentMsg.MessageID,
	})
	if err != nil {
		log.Printf("Failed to log send_text_to_channel action for user %d: %v", userID, err)
	}

	// Send confirmation using localized message
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)
	msg := locales.GetMessage(localizer, "MsgPostSentToChannel", nil, nil)
	return h.sendSuccess(ctx, bot, chatID, msg)
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
	userID := message.From.ID
	isAdmin := false
	if h.suggestionManager != nil {
		var checkErr error
		isAdmin, checkErr = h.suggestionManager.IsAdmin(ctx, userID)
		if checkErr != nil {
			log.Printf("Error checking admin status for user %d in HandlePhoto: %v. Assuming non-admin.", userID, checkErr)
			isAdmin = false
		}
	} else {
		log.Printf("Warning: Suggestion manager is nil in HandlePhoto, cannot check admin status for user %d", userID)
	}
	if !isAdmin {
		log.Printf("User %d (not admin) attempted to send photo directly.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		return h.sendError(ctx, bot, message.Chat.ID, errors.New(msg))
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
		SenderID:       userID,
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
		log.Printf("Failed attempt to log photo post to DB from user %d. Error: %v", userID, err)
		// Potentially send to Sentry
	}

	// Update user information
	err = h.userRepo.UpdateUser(ctx, userID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_photo_to_channel")
	if err != nil {
		log.Printf("Failed to update user info after sending photo for user %d: %v", userID, err)
		// Potentially send to Sentry
	}

	// Log photo sending action
	err = h.actionLogger.LogUserAction(userID, "send_photo_to_channel", map[string]interface{}{
		"chat_id":             message.Chat.ID,
		"original_message_id": message.MessageID,
		"channel_message_id":  sentMsgID.MessageID,
		"caption_used":        caption,
	})
	if err != nil {
		log.Printf("Failed to log send_photo_to_channel action for user %d: %v", userID, err)
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
