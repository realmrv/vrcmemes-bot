package handlers

import (
	"fmt"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// sendError sends an error message to the user
func (h *MessageHandler) sendError(ctx *th.Context, chatID int64, err error) error {
	_, err = ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		fmt.Sprintf(msgErrorSendingMessage, err.Error()),
	))
	return err
}

// sendSuccess sends a success message to the user
func (h *MessageHandler) sendSuccess(ctx *th.Context, chatID int64, message string) error {
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		message,
	))
	return err
}

// isUserAdmin checks if the user is an administrator of the channel
func (h *MessageHandler) isUserAdmin(ctx *th.Context, userID int64) (bool, error) {
	chatMember, err := ctx.Bot().GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: tu.ID(h.channelID),
		UserID: userID,
	})
	if err != nil {
		return false, err
	}
	status := chatMember.MemberStatus()
	return status == "creator" || status == "administrator", nil
}

// GetChannelID returns the channel ID
func (h *MessageHandler) GetChannelID() int64 {
	return h.channelID
}
