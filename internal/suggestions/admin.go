package suggestions

import (
	"context"
	"fmt"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"log"
	"vrcmemes-bot/internal/locales"
)

// IsAdmin is now defined correctly in manager.go

// HandleReviewCommand handles the /review command by initiating a review session.
func (m *Manager) HandleReviewCommand(ctx context.Context, update telego.Update) error {
	chatID := update.Message.Chat.ID
	adminID := update.Message.From.ID
	log.Printf("[/review] command received from admin %d in chat %d", adminID, chatID)

	// Determine language (use default for now)
	lang := locales.DefaultLanguage
	if update.Message.From != nil && update.Message.From.LanguageCode != "" {
		// lang = update.Message.From.LanguageCode // TODO: Use admin preference
	}
	localizer := locales.NewLocalizer(lang)

	// Send confirmation message
	startMsg := locales.GetMessage(localizer, "MsgReviewSessionStarting", nil, nil)
	_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), startMsg))
	if err != nil {
		log.Printf("Error sending review start confirmation message to %d: %v", chatID, err)
		// Don't return here, try starting the session anyway
	}

	err = m.startReviewSession(ctx, adminID, chatID)
	if err != nil {
		log.Printf("Error starting review session for admin %d: %v", adminID, err)
		// Send localized error message to the admin
		errorMsg := locales.GetMessage(localizer, "MsgReviewErrorStartingSession", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("Error sending review session start error message to %d: %v", chatID, sendErr)
		}
	}
	return err // Return the error from startReviewSession (or nil if successful)
}

// startReviewSession starts a new review session for an admin.
func (m *Manager) startReviewSession(ctx context.Context, adminID, chatID int64) error {
	const batchSize = 5                                               // Number of suggestions to review at once
	suggestions, _, err := m.GetPendingSuggestions(ctx, batchSize, 0) // Fetch first batch
	if err != nil {
		return fmt.Errorf("failed to get pending suggestions: %w", err)
	}

	// TODO: Determine language from adminID/chatID preferences?
	localizer := locales.NewLocalizer(locales.DefaultLanguage)

	if len(suggestions) == 0 {
		queueEmptyMsg := locales.GetMessage(localizer, "MsgReviewQueueIsEmpty", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), queueEmptyMsg))
		return err
	}

	session := &ReviewSession{
		AdminID:      adminID, // Store the admin ID
		ReviewChatID: chatID,  // Store the chat ID where the review started
		Suggestions:  suggestions,
		CurrentIndex: 0,
	}

	m.reviewSessionsMutex.Lock()
	m.reviewSessions[adminID] = session
	m.reviewSessionsMutex.Unlock()

	// Send the first suggestion for review
	return m.SendReviewMessage(ctx, chatID, adminID, 0)
}
