package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
	tu "github.com/mymmrac/telego/telegoutil"
)

// HandleText handles incoming text messages (excluding commands).
// It checks if the user is an admin and then processes the text for publishing or setting as caption.
func (h *MessageHandler) HandleText(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	userID := message.From.ID
	chatID := message.Chat.ID
	localizer := h.getLocalizer(message.From) // Use helper

	// Check if we are waiting for a caption from this chat
	if _, waiting := h.waitingForCaption.Load(chatID); waiting {
		captionText := message.Text
		log.Printf("[Cmd:CaptionReply User:%d Chat:%d] Received caption text \"%s\"", userID, chatID, captionText)

		// Store the caption
		_, exists := h.activeCaptions.Load(chatID)
		h.activeCaptions.Store(chatID, captionText)
		h.waitingForCaption.Delete(chatID) // Clear the waiting state

		var confirmationMsg string
		if exists {
			confirmationMsg = locales.GetMessage(localizer, "MsgCaptionOverwriteConfirmation", nil, nil)
		} else {
			confirmationMsg = locales.GetMessage(localizer, "MsgCaptionSetConfirmation", nil, nil)
		}

		// Record activity (isAdmin assumed false here, as this is caption reply)
		h.recordUserActivity(ctx, message.From, ActionSetCaptionReply, false, map[string]interface{}{
			"chat_id": chatID,
			"caption": captionText,
		})

		return h.sendSuccess(ctx, bot, chatID, confirmationMsg)
	}

	// If not waiting for caption, proceed with admin check and potential direct posting
	// --- Admin Check ---
	isAdmin, err := h.checkAdmin(ctx, userID)
	if err != nil {
		// If checkAdmin failed significantly (e.g., manager nil), send generic error
		if errors.Is(err, errors.New("suggestion manager not initialized")) {
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
		}
		// Otherwise, assume non-admin
		isAdmin = false
	}

	if !isAdmin {
		log.Printf("[HandleText User:%d] Non-admin attempted to send text directly.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		// Don't record activity for failed attempts
		return h.sendError(ctx, bot, chatID, errors.New(msg))
	}
	// --- End Admin Check ---

	// Admin is sending text directly for publishing
	log.Printf("[HandleText Admin:%d] Sending text message to channel %d", userID, h.channelID)
	textToPublish := message.Text

	// TODO: Consider if admins should be able to set caption with simple text? Unlikely.

	sentMsg, err := bot.SendMessage(ctx, tu.Message(tu.ID(h.channelID), textToPublish))
	if err != nil {
		// Error sending to channel - report back to admin
		// Log the specific error
		log.Printf("[HandleText Admin:%d] Failed to send text to channel %d: %v", userID, h.channelID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorSendToChannel", nil, nil)
		// Don't use h.sendError directly, as it returns the original error. We want to show the localized message.
		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		return err // Return original error for Sentry etc.
	}

	// Log the successful post
	log.Printf("[HandleText Admin:%d] Successfully sent text message %d to channel %d", userID, sentMsg.MessageID, h.channelID)

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
		log.Printf("[HandleText Admin:%d] Failed attempt to log text post to DB. Error: %v", userID, err)
		// Log only, don't fail the operation for the user
	}

	// Record activity
	h.recordUserActivity(ctx, message.From, ActionSendTextToChannel, isAdmin, map[string]interface{}{
		"chat_id":            chatID,
		"text":               message.Text,
		"channel_message_id": sentMsg.MessageID,
	})

	// Send confirmation using localized message
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

	localizer := h.getLocalizer(message.From) // Use helper

	// Check admin rights before posting photo to channel
	userID := message.From.ID
	isAdmin, err := h.checkAdmin(ctx, userID) // Use helper
	if err != nil {
		// If checkAdmin failed significantly, send generic error
		if errors.Is(err, errors.New("suggestion manager not initialized")) {
			errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
			return h.sendError(ctx, bot, message.Chat.ID, errors.New(errorMsg))
		}
		// Otherwise, assume non-admin
		isAdmin = false
	}

	if !isAdmin {
		log.Printf("[HandlePhoto User:%d] Non-admin attempted to send photo directly.", userID)
		msg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		// Don't record activity
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
		// Error sending to channel - report back to admin
		log.Printf("[HandlePhoto Admin:%d] Failed to copy photo message %d to channel %d: %v", userID, message.MessageID, h.channelID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorSendToChannel", nil, nil)
		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), errorMsg))
		return err // Return original error for Sentry etc.
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
		log.Printf("[HandlePhoto Admin:%d] Failed attempt to log photo post to DB. Error: %v", userID, err)
		// Log only, don't fail the operation for the user
	}

	// Record activity
	h.recordUserActivity(ctx, message.From, ActionSendPhotoToChannel, isAdmin, map[string]interface{}{
		"chat_id":             message.Chat.ID,
		"original_message_id": message.MessageID,
		"channel_message_id":  sentMsgID.MessageID,
		"caption_used":        caption,
	})

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
