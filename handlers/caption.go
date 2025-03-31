package handlers

import (
	"fmt"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// HandleCaption handles the /caption command
func (h *MessageHandler) HandleCaption(ctx *th.Context, message telego.Message) error {
	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRightsCaption)
	}

	h.waitingForCaption.Store(message.Chat.ID, true)
	return h.sendSuccess(ctx, message.Chat.ID, msgCaptionPrompt)
}

// HandleShowCaption handles the /showcaption command
func (h *MessageHandler) HandleShowCaption(ctx *th.Context, message telego.Message) error {
	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRightsViewCaption)
	}

	caption, exists := h.GetActiveCaption(message.Chat.ID)
	if !exists {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoCaptionSet)
	}
	return h.sendSuccess(ctx, message.Chat.ID, fmt.Sprintf(msgCurrentCaption, caption))
}

// HandleClearCaption handles the /clearcaption command
func (h *MessageHandler) HandleClearCaption(ctx *th.Context, message telego.Message) error {
	isAdmin, err := h.isUserAdmin(ctx, message.From.ID)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
	}
	if !isAdmin {
		return h.sendSuccess(ctx, message.Chat.ID, msgNoAdminRightsCaption)
	}

	h.clearActiveCaption(message.Chat.ID)
	return h.sendSuccess(ctx, message.Chat.ID, msgCaptionCleared)
}

// GetActiveCaption returns the active caption for a user
func (h *MessageHandler) GetActiveCaption(chatID int64) (string, bool) {
	if caption, exists := h.activeCaptions.Load(chatID); exists {
		return caption.(string), true
	}
	return "", false
}

// setActiveCaption sets the active caption for a user
func (h *MessageHandler) setActiveCaption(chatID int64, caption string) {
	h.activeCaptions.Store(chatID, caption)
}

// clearActiveCaption removes the active caption for a user
func (h *MessageHandler) clearActiveCaption(chatID int64) {
	h.activeCaptions.Delete(chatID)
}

// StoreMediaGroupCaption stores caption for a media group
func (h *MessageHandler) StoreMediaGroupCaption(mediaGroupID string, caption string) {
	h.mediaGroupCaptions.Store(mediaGroupID, caption)
}
