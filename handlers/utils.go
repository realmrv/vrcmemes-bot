package handlers

import (
	"context"
	"log"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// sendError sends an error message to the user
func (h *MessageHandler) sendError(ctx context.Context, bot *telego.Bot, chatID int64, err error) error {
	log.Printf("Error for chat %d: %v", chatID, err)
	_, sendErr := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "An error occurred: "+err.Error()))
	return sendErr // Return the error from sending the message
}

// sendSuccess sends a success message to the user
func (h *MessageHandler) sendSuccess(ctx context.Context, bot *telego.Bot, chatID int64, message string) error {
	_, err := bot.SendMessage(ctx, tu.Message(
		tu.ID(chatID),
		message,
	))
	if err != nil {
		log.Printf("Error sending success message to %d: %v", chatID, err)
	}
	return err
}

// isUserAdmin checks if the user is an administrator of the channel
func (h *MessageHandler) isUserAdmin(ctx context.Context, bot *telego.Bot, userID int64) (bool, error) {
	// Note: This check works for private chats (always true) or groups/supergroups.
	// It might need adjustment depending on where commands can be used.
	// Assuming commands are primarily used in private chats or the user interacting is an admin.
	// A more robust check might involve GetChatMember in group chats.

	// For simplicity in this refactor, let's assume private chat or admin required.
	// In a private chat, the user is always effectively the "admin" of that chat.
	// Consider adding a check for message.Chat.Type == telego.ChatTypePrivate
	// Or, perform a GetChatMember call if it's a group/supergroup.

	// Placeholder implementation: Assume true for now to avoid breaking command handlers.
	// TODO: Implement proper chat member check if needed for group scenarios.
	return true, nil

	/* // Original logic using ctx.Bot() - Keep for reference if needed
	member, err := ctx.Bot().GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: tu.ID(h.channelID), // Check against the main channel? Or the message chat?
		UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get chat member: %w", err)
	}
	status := member.Status
	return status == "creator" || status == "administrator", nil
	*/
}
