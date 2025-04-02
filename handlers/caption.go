package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/mymmrac/telego"
	// th "github.com/mymmrac/telego/telegohandler" // No longer needed
)

// HandleCaption handles the /caption command
func (h *MessageHandler) HandleCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	h.waitingForCaption.Store(message.Chat.ID, true)
	return h.sendSuccess(ctx, bot, message.Chat.ID, msgCaptionPrompt)
}

// HandleShowCaption handles the /showcaption command
func (h *MessageHandler) HandleShowCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	caption, exists := h.GetActiveCaption(message.Chat.ID)
	if !exists {
		return h.sendSuccess(ctx, bot, message.Chat.ID, msgShowCaptionInactive)
	}
	return h.sendSuccess(ctx, bot, message.Chat.ID, fmt.Sprintf(msgShowCaptionActive, caption))
}

// HandleClearCaption handles the /clearcaption command
func (h *MessageHandler) HandleClearCaption(ctx context.Context, bot *telego.Bot, message telego.Message) error {
	h.clearActiveCaption(message.Chat.ID)

	// Log clear caption action
	err := h.actionLogger.LogUserAction(message.From.ID, "command_clearcaption", map[string]interface{}{
		"chat_id": message.Chat.ID,
	})
	if err != nil {
		log.Printf("Failed to log clear caption command: %v", err)
	}

	return h.sendSuccess(ctx, bot, message.Chat.ID, msgCaptionCleared)
}

// GetActiveCaption returns the active caption for a chat
func (h *MessageHandler) GetActiveCaption(chatID int64) (string, bool) {
	if caption, ok := h.activeCaptions.Load(chatID); ok {
		if capStr, okStr := caption.(string); okStr {
			return capStr, true
		}
	}
	return "", false
}

// setActiveCaption sets the active caption for a chat
func (h *MessageHandler) setActiveCaption(chatID int64, caption string) {
	h.activeCaptions.Store(chatID, caption)
}

// clearActiveCaption removes the active caption for a chat
func (h *MessageHandler) clearActiveCaption(chatID int64) {
	h.activeCaptions.Delete(chatID)
}

// StoreMediaGroupCaption stores a caption associated with a media group ID
func (h *MessageHandler) StoreMediaGroupCaption(groupID, caption string) {
	if groupID != "" && caption != "" {
		h.mediaGroupCaptions.Store(groupID, caption)
	}
}
