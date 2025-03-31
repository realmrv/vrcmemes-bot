package handlers

import (
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

	_, err = ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(h.channelID),
		message.Text,
	).WithParseMode(telego.ModeHTML),
	)
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
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
	_, err = ctx.Bot().CopyMessage(ctx, &telego.CopyMessageParams{
		ChatID:     tu.ID(h.channelID),
		FromChatID: tu.ID(message.Chat.ID),
		MessageID:  message.MessageID,
		Caption:    caption,
	})
	if err != nil {
		return h.sendError(ctx, message.Chat.ID, err)
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

	if caption != "" {
		return h.sendSuccess(ctx, message.Chat.ID, msgMediaGroupWithCaption)
	}
	return h.sendSuccess(ctx, message.Chat.ID, msgMediaGroupSuccess)
}
