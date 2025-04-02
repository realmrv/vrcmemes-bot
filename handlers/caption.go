package handlers

import (
	"context"
	"log"

	"github.com/mymmrac/telego"

	"vrcmemes-bot/pkg/locales" // Import locales package
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
)

// HandleCaption handles the /caption command.
// It sets a flag indicating the bot is waiting for the user's next text message to be used as the active caption,
// and sends a message asking the user for the caption text.
func (h *MessageHandler) HandleCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// Set the state to wait for the next text message from this chat as caption
	h.waitingForCaption.Store(message.Chat.ID, true)

	// Log the initiation of the caption setting process
	err := h.actionLogger.LogUserAction(message.From.ID, "command_caption_start", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		log.Printf("Failed to log /caption command start for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Ask the user to provide the caption text using localized message
	msg := locales.GetMessage(localizer, "MsgCaptionAskForInput", nil, nil)
	return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
}

// HandleShowCaption handles the /showcaption command.
// It retrieves the currently active caption for the chat and sends it to the user.
// If no caption is set, it informs the user.
func (h *MessageHandler) HandleShowCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, exists := h.GetActiveCaption(message.Chat.ID)

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	// Log the action of showing the caption
	err := h.actionLogger.LogUserAction(message.From.ID, "command_showcaption", map[string]interface{}{
		"chat_id":        message.Chat.ID,
		"caption_exists": exists,
		"caption":        caption, // Log the caption if it exists
	})
	if err != nil {
		log.Printf("Failed to log /showcaption command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	var msg string
	if !exists {
		msg = locales.GetMessage(localizer, "MsgCaptionShowEmpty", nil, nil)
	} else {
		msg = locales.GetMessage(localizer, "MsgCaptionShowCurrent", map[string]interface{}{
			"Caption": caption,
		}, nil)
	}
	return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
}

// HandleClearCaption handles the /clearcaption command.
// It removes the active caption associated with the chat and confirms the action to the user.
func (h *MessageHandler) HandleClearCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	// Retrieve caption before clearing for logging purposes
	_, existed := h.GetActiveCaption(message.Chat.ID)

	h.clearActiveCaption(message.Chat.ID)

	// Log clear caption action
	err := h.actionLogger.LogUserAction(message.From.ID, "command_clearcaption", map[string]interface{}{
		"chat_id":         message.Chat.ID,
		"caption_existed": existed,
	})
	if err != nil {
		log.Printf("Failed to log /clearcaption command for user %d: %v", message.From.ID, err)
		// Potentially send to Sentry
	}

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	if message.From != nil && message.From.LanguageCode != "" {
		// lang = message.From.LanguageCode
	}
	localizer := locales.NewLocalizer(lang)

	msg := locales.GetMessage(localizer, "MsgCaptionClearedConfirmation", nil, nil)
	return h.sendSuccess(ctx, bot, message.Chat.ID, msg)
}

// GetActiveCaption retrieves the currently active caption for a specific chat ID.
// It returns the caption string and a boolean indicating if a caption was found.
// This uses a sync.Map for thread-safe access.
func (h *MessageHandler) GetActiveCaption(chatID int64) (string, bool) {
	caption, ok := h.activeCaptions.Load(chatID)
	if !ok {
		return "", false // No caption found for this chat ID
	}

	capStr, okStr := caption.(string)
	if !okStr {
		// This indicates a potential issue, the value stored was not a string
		log.Printf("WARN: Invalid type stored in activeCaptions for chat ID %d: expected string, got %T", chatID, caption)
		h.activeCaptions.Delete(chatID) // Clean up invalid entry
		return "", false
	}

	return capStr, true
}

// setActiveCaption stores the provided caption string as the active caption for the given chat ID.
// It uses a sync.Map for thread-safe storage.
func (h *MessageHandler) setActiveCaption(chatID int64, caption string) {
	if caption == "" {
		// Setting an empty caption is equivalent to clearing it
		h.clearActiveCaption(chatID)
		return
	}
	h.activeCaptions.Store(chatID, caption)
	log.Printf("Active caption set for chat %d", chatID) // Log setting action
}

// clearActiveCaption removes the active caption associated with the given chat ID.
// It uses a sync.Map for thread-safe deletion.
func (h *MessageHandler) clearActiveCaption(chatID int64) {
	h.activeCaptions.Delete(chatID)
	log.Printf("Active caption cleared for chat %d", chatID) // Log clearing action
}

// StoreMediaGroupCaption stores a caption associated with a specific media group ID.
// This is likely used to apply captions to media groups received shortly after text messages.
// It uses a sync.Map for thread-safe storage.
func (h *MessageHandler) StoreMediaGroupCaption(groupID, caption string) {
	if groupID == "" {
		log.Println("WARN: StoreMediaGroupCaption called with empty groupID")
		return
	}
	if caption == "" {
		log.Println("WARN: StoreMediaGroupCaption called with empty caption")
		// Optionally delete if an empty caption means removal: h.mediaGroupCaptions.Delete(groupID)
		return
	}
	h.mediaGroupCaptions.Store(groupID, caption)
	log.Printf("Caption stored for media group %s", groupID)
}
