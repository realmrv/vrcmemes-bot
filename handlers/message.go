package handlers

import (
	"context"
	"log"
	"time"

	"vrcmemes-bot/database"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// HandleText handles text messages
func (h *MessageHandler) HandleText(ctx *th.Context, message telego.Message) error {
	if message.Text == "" || message.Text == "/start" {
		return nil
	}

	if _, waiting := h.waitingForCaption.Load(message.Chat.ID); waiting {
		_, hadPreviousCaption := h.GetActiveCaption(message.Chat.ID)
		h.setActiveCaption(message.Chat.ID, message.Text)
		h.waitingForCaption.Delete(message.Chat.ID)

		// Update user information
		isAdmin, _ := h.isUserAdmin(ctx, message.From.ID)
		err := database.UpdateUser(
			h.db,
			message.From.ID,
			message.From.Username,
			message.From.FirstName,
			message.From.LastName,
			isAdmin,
			"set_caption",
		)
		if err != nil {
			log.Printf("Failed to update user info: %v", err)
		}

		// Log caption setting action
		_, err = h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
			"user_id": message.From.ID,
			"action":  "set_caption",
			"details": map[string]interface{}{
				"chat_id":   message.Chat.ID,
				"caption":   message.Text,
				"overwrite": hadPreviousCaption,
			},
			"time": time.Now(),
		})
		if err != nil {
			log.Printf("Failed to log caption action: %v", err)
		}

		if hadPreviousCaption {
			return h.sendSuccess(ctx, message.Chat.ID, msgCaptionOverwrite)
		}
		return h.sendSuccess(ctx, message.Chat.ID, msgCaptionSaved)
	}

	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRights)
	}

	// SendMessage returns the sent message, capture it
	sentMsg, err := ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(h.channelID),
		message.Text,
	).WithParseMode(telego.ModeHTML),
	)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	publishedTime := time.Now()

	// Create log entry for the text message
	logEntry := PostLog{
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        message.Text, // For text messages, the text is the content
		MessageType:    "text",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsg.MessageID, // Use the returned message ID
	}

	// Log to MongoDB
	if err := h.LogPublishedPost(logEntry); err != nil {
		// Error is logged within LogPublishedPost
		log.Printf("Failed attempt to log text post to DB from user %d", message.From.ID)
	}

	// Update user information
	err = database.UpdateUser(
		h.db,
		message.From.ID,
		message.From.Username,
		message.From.FirstName,
		message.From.LastName,
		isAdmin,
		"send_text",
	)
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log text message sending action
	_, err = h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "send_text",
		"details": map[string]interface{}{
			"chat_id": message.Chat.ID,
			"text":    message.Text,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log text message action: %v", err)
	}

	return h.sendSuccess(ctx, message.Chat.ID, msgPostSuccess)
}

// HandlePhoto handles photo messages
func (h *MessageHandler) HandlePhoto(ctx *th.Context, message telego.Message) error {
	if message.Photo == nil {
		return nil
	}

	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRightsPhoto)
	}

	caption, _ := h.GetActiveCaption(message.Chat.ID)
	// CopyMessage returns the sent message ID, capture it
	sentMsgID, err := ctx.Bot().CopyMessage(ctx, &telego.CopyMessageParams{
		ChatID:     tu.ID(h.channelID),
		FromChatID: tu.ID(message.Chat.ID),
		MessageID:  message.MessageID,
		Caption:    caption,
	})
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	publishedTime := time.Now()

	// Create log entry for the photo
	logEntry := PostLog{
		SenderID:       message.From.ID,
		SenderUsername: message.From.Username,
		Caption:        caption, // The actual caption used
		MessageType:    "photo",
		ReceivedAt:     time.Unix(int64(message.Date), 0),
		PublishedAt:    publishedTime,
		ChannelID:      h.channelID,
		ChannelPostID:  sentMsgID.MessageID, // Use the returned message ID
	}

	// Log to MongoDB
	if err := h.LogPublishedPost(logEntry); err != nil {
		// Error is logged within LogPublishedPost
		log.Printf("Failed attempt to log photo post to DB from user %d", message.From.ID)
	}

	// Update user information
	err = database.UpdateUser(
		h.db,
		message.From.ID,
		message.From.Username,
		message.From.FirstName,
		message.From.LastName,
		isAdmin,
		"send_photo",
	)
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log photo sending action
	_, err = h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "send_photo",
		"details": map[string]interface{}{
			"chat_id":    message.Chat.ID,
			"message_id": message.MessageID,
			"caption":    caption,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log photo action: %v", err)
	}

	if caption != "" {
		return h.sendSuccess(ctx, message.Chat.ID, msgPhotoWithCaption)
	}
	return h.sendSuccess(ctx, message.Chat.ID, msgPhotoSuccess)
}

// HandleMediaGroup handles media group messages
func (h *MessageHandler) HandleMediaGroup(ctx *th.Context, message telego.Message) error {
	if message.MediaGroupID == "" {
		return nil
	}

	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRightsMedia)
	}

	caption, _ := h.GetActiveCaption(message.Chat.ID)
	if caption != "" {
		h.mediaGroupCaptions.Store(message.MediaGroupID, caption)
	}

	var inputMedia []telego.InputMedia
	if message.Photo != nil {
		photo := message.Photo[len(message.Photo)-1]
		mediaPhoto := &telego.InputMediaPhoto{
			Type:  "photo",
			Media: telego.InputFile{FileID: photo.FileID},
		}
		if caption != "" {
			mediaPhoto.Caption = caption
		}
		inputMedia = append(inputMedia, mediaPhoto)
	}

	_, err = ctx.Bot().SendMediaGroup(ctx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(h.channelID),
		Media:  inputMedia,
	})
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}

	// Update user information
	err = database.UpdateUser(
		h.db,
		message.From.ID,
		message.From.Username,
		message.From.FirstName,
		message.From.LastName,
		isAdmin,
		"send_media_group",
	)
	if err != nil {
		log.Printf("Failed to update user info: %v", err)
	}

	// Log media group sending action
	_, err = h.db.Collection("user_actions").InsertOne(context.Background(), map[string]interface{}{
		"user_id": message.From.ID,
		"action":  "send_media_group",
		"details": map[string]interface{}{
			"chat_id":        message.Chat.ID,
			"message_id":     message.MessageID,
			"media_group_id": message.MediaGroupID,
			"caption":        caption,
		},
		"time": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to log media group action: %v", err)
	}

	if caption != "" {
		return h.sendSuccess(ctx, message.Chat.ID, msgMediaGroupWithCaption)
	}
	return h.sendSuccess(ctx, message.Chat.ID, msgMediaGroupSuccess)
}

// LogPublishedPost saves the post log entry to the database.
func (h *MessageHandler) LogPublishedPost(logEntry PostLog) error {
	collection := h.db.Collection("post_logs")
	_, err := collection.InsertOne(context.Background(), logEntry)
	if err != nil {
		log.Printf("Error logging published post to MongoDB: %v", err)
		// Depending on requirements, you might want to handle this error differently,
		// e.g., retry, log to a fallback, or ignore.
	}
	return err // Return the error status
}
