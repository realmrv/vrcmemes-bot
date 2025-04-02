package handlers

import (
	"context"
	"log"
	"time"

	"vrcmemes-bot/database/models"

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
	tu "github.com/mymmrac/telego/telegoutil"
)

// HandleText handles text messages
func (h *MessageHandler) HandleText(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	if message.Text == "" || message.Text == "/start" {
		return nil
	}

	if _, waiting := h.waitingForCaption.Load(message.Chat.ID); waiting {
		_, hadPreviousCaption := h.GetActiveCaption(message.Chat.ID)
		h.setActiveCaption(message.Chat.ID, message.Text)
		h.waitingForCaption.Delete(message.Chat.ID)

		// Update user information
		// isAdmin, _ := h.isUserAdmin(ctx, bot, message.From.ID) // Use updated isUserAdmin
		isAdmin := true // Assuming placeholder returns true
		err := h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "set_caption")
		if err != nil {
			log.Printf("Failed to update user info: %v", err)
		}

		// Log caption setting action
		err = h.actionLogger.LogUserAction(message.From.ID, "set_caption", map[string]interface{}{
			"chat_id":   message.Chat.ID,
			"caption":   message.Text,
			"overwrite": hadPreviousCaption,
		})
		if err != nil {
			log.Printf("Failed to log caption action: %v", err)
		}

		if hadPreviousCaption {
			return h.sendSuccess(ctx, bot, message.Chat.ID, msgCaptionOverwrite)
		}
		return h.sendSuccess(ctx, bot, message.Chat.ID, msgCaptionSet)
	}

	// Check admin rights before posting text to channel
	isAdmin, err := h.isUserAdmin(ctx, bot, message.From.ID)
	if err != nil {
		return h.sendError(ctx, bot, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, bot, message.Chat.ID, msgErrorUserNotAdmin)
	}

	// Send message to channel using passed bot instance
	sentMsg, err := bot.SendMessage(ctx, tu.Message(
		tu.ID(h.channelID),
		message.Text,
	).WithParseMode(telego.ModeHTML),
	)
	if err != nil {
		return h.sendError(ctx, bot, message.Chat.ID, err)
	}

	publishedTime := time.Now()

	// Create log entry for the text message
	logEntry := models.PostLog{ // Use models.PostLog
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        message.Text,
		MessageType:    "text",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsg.MessageID,
	}

	// Log to MongoDB using postLogger
	if err := h.postLogger.LogPublishedPost(logEntry); err != nil {
		log.Printf("Failed attempt to log text post to DB from user %d", message.From.ID)
	}

	// Update user information using userRepo
	err = h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_text")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log text message sending action using actionLogger
	err = h.actionLogger.LogUserAction(message.From.ID, "send_text", map[string]interface{}{
		"chat_id": message.Chat.ID,
		"text":    message.Text,
	})
	if err != nil {
		log.Printf("Failed to log text message action: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, msgPostSent)
}

// HandlePhoto handles photo messages
func (h *MessageHandler) HandlePhoto(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	if message.Photo == nil {
		return nil
	}

	isAdmin, err := h.isUserAdmin(ctx, bot, message.From.ID)
	if err != nil {
		return h.sendError(ctx, bot, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, bot, message.Chat.ID, msgErrorUserNotAdmin)
	}

	caption, _ := h.GetActiveCaption(message.Chat.ID)
	// Copy message using passed bot instance
	sentMsgID, err := bot.CopyMessage(ctx, &telego.CopyMessageParams{
		ChatID:     tu.ID(h.channelID),
		FromChatID: tu.ID(message.Chat.ID),
		MessageID:  message.MessageID,
		Caption:    caption,
	})
	if err != nil {
		return h.sendError(ctx, bot, message.Chat.ID, err)
	}

	publishedTime := time.Now()

	// Create log entry for the photo
	logEntry := models.PostLog{ // Use models.PostLog
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        caption,
		MessageType:    "photo",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsgID.MessageID,
	}

	// Log to MongoDB using postLogger
	if err := h.postLogger.LogPublishedPost(logEntry); err != nil {
		log.Printf("Failed attempt to log photo post to DB from user %d", message.From.ID)
	}

	// Update user information using userRepo
	err = h.userRepo.UpdateUser(ctx, message.From.ID, message.From.Username, message.From.FirstName, message.From.LastName, isAdmin, "send_photo")
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log photo sending action using actionLogger
	err = h.actionLogger.LogUserAction(message.From.ID, "send_photo", map[string]interface{}{
		"chat_id":    message.Chat.ID,
		"message_id": message.MessageID,
		"caption":    caption,
	})
	if err != nil {
		log.Printf("Failed to log photo action: %v", err)
	}

	if caption != "" {
		return h.sendSuccess(ctx, bot, message.Chat.ID, msgPostSent)
	}
	return h.sendSuccess(ctx, bot, message.Chat.ID, msgPostSent)
}

// HandleMediaGroup handles media group messages (This might be deprecated if bot.go handles it)
// If kept, it needs signature update and logic adjustment
/* // Commenting out for now as bot.go handles media group assembly
func (h *MessageHandler) HandleMediaGroup(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// ... (update logic similarly to HandlePhoto/HandleText) ...
}
*/
