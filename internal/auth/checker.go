package auth

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mymmrac/telego"
)

// AdminChecker handles checking user admin status against a configured channel.
type AdminChecker struct {
	bot             *telego.Bot
	targetChannelID int64
}

// NewAdminChecker creates a new AdminChecker.
// It requires a non-nil bot instance and a non-zero target channel ID.
func NewAdminChecker(bot *telego.Bot, channelID int64) (*AdminChecker, error) {
	if bot == nil {
		return nil, fmt.Errorf("telego bot instance cannot be nil")
	}
	if channelID == 0 {
		return nil, fmt.Errorf("target channel ID cannot be zero")
	}
	return &AdminChecker{
		bot:             bot,
		targetChannelID: channelID,
	}, nil
}

// IsAdmin checks if a user is an administrator or creator in the target channel
// configured in the AdminChecker.
func (ac *AdminChecker) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	member, err := ac.bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: telego.ChatID{ID: ac.targetChannelID}, // Use stored channel ID
		UserID: userID,
	})
	if err != nil {
		// A user not found in the channel is simply not an admin.
		// API errors (network, permissions) should be returned.
		if strings.Contains(strings.ToLower(err.Error()), "user not found") {
			return false, nil
		}
		// Log other potential errors but don't expose details unless necessary
		log.Printf("[AdminCheck User:%d Channel:%d] Error checking chat member: %v. Assuming non-admin.", userID, ac.targetChannelID, err)
		return false, fmt.Errorf("failed to get chat member info: %w", err)
	}

	status := member.MemberStatus()
	isAdminStatus := status == telego.MemberStatusCreator || status == telego.MemberStatusAdministrator
	return isAdminStatus, nil
}
