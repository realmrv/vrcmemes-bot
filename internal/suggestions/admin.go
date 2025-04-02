package suggestions

import (
	"context"
	"fmt"
	"log"
	"time"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// IsAdmin checks if a user ID belongs to an administrator of the target channel.
// Uses cached results if available and not expired.
func (m *Manager) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	m.adminCacheMutex.RLock()
	isCached := false
	if time.Since(m.adminCacheTime) < m.adminCacheTTL {
		for _, admin := range m.adminCache {
			if user := admin.MemberUser(); user.ID != 0 && user.ID == userID {
				isCached = true
				break
			}
		}
	}
	m.adminCacheMutex.RUnlock()

	if isCached {
		return true, nil
	}

	m.adminCacheMutex.Lock()
	defer m.adminCacheMutex.Unlock()

	if time.Since(m.adminCacheTime) < m.adminCacheTTL {
		for _, admin := range m.adminCache {
			if user := admin.MemberUser(); user.ID != 0 && user.ID == userID {
				return true, nil
			}
		}
	}

	admins, err := m.bot.GetChatAdministrators(ctx, &telego.GetChatAdministratorsParams{
		ChatID: telego.ChatID{ID: m.targetChannelID},
	})
	if err != nil {
		return false, fmt.Errorf("failed to get chat administrators: %w", err)
	}

	m.adminCache = admins
	m.adminCacheTime = time.Now()

	for _, admin := range admins {
		if user := admin.MemberUser(); user.ID != 0 && user.ID == userID {
			return true, nil
		}
	}

	return false, nil
}

// HandleReviewCommand handles the /review command.
func (m *Manager) HandleReviewCommand(ctx context.Context, update telego.Update) error {
	chatID := update.Message.Chat.ID
	log.Printf("[/review] command received from chat %d (Implementation Pending)", chatID)

	_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "Review command received, starting review session...")) // Placeholder message
	if err != nil {
		log.Printf("Error sending review start message: %v", err)
	}
	// Placeholder: Call a function to start the actual review session
	err = m.startReviewSession(ctx, update.Message.From.ID, chatID)
	if err != nil {
		log.Printf("Error starting review session for user %d: %v", update.Message.From.ID, err)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "Error starting review session."))
	}
	return err // Return error from starting session if implemented
}

// startReviewSession starts a new review session for an admin.
func (m *Manager) startReviewSession(ctx context.Context, adminID, chatID int64) error {
	const batchSize = 5                                               // Number of suggestions to review at once
	suggestions, _, err := m.GetPendingSuggestions(ctx, batchSize, 0) // Fetch first batch
	if err != nil {
		return fmt.Errorf("failed to get pending suggestions: %w", err)
	}

	if len(suggestions) == 0 {
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), locales.MsgReviewQueueIsEmpty))
		return err
	}

	session := &ReviewSession{
		Suggestions:  suggestions,
		CurrentIndex: 0,
	}

	m.reviewSessionsMutex.Lock()
	m.reviewSessions[adminID] = session
	m.reviewSessionsMutex.Unlock()

	// Send the first suggestion for review
	return m.SendReviewMessage(ctx, chatID, adminID, 0)
}
