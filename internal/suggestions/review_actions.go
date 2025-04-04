package suggestions

import (
	"context"
	"fmt"
	"log"

	"vrcmemes-bot/database/models"
	"vrcmemes-bot/pkg/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// handleApproveAction processes the approval of a suggestion.
func (m *Manager) handleApproveAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, index int, reviewMessageID int) error {
	suggestionToPublish := session.Suggestions[index]

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	// TODO: Get admin lang pref
	localizer := locales.NewLocalizer(lang)

	err := m.publishSuggestion(ctx, suggestionToPublish)
	if err != nil {
		log.Printf("[handleApproveAction] Failed to publish suggestion %s: %v", suggestionToPublish.ID.Hex(), err)
		errorMsg := locales.GetMessage(localizer, "MsgReviewErrorDuringPublishing", nil, nil)
		_ = m.answerCallbackQuery(ctx, queryID, errorMsg, true)
		return err
	}

	err = m.UpdateSuggestionStatus(ctx, suggestionToPublish.ID, models.StatusApproved, adminID)
	if err != nil {
		log.Printf("[handleApproveAction] Failed to update suggestion %s status to approved after publishing: %v", suggestionToPublish.ID.Hex(), err)
		// Send confirmation but mention DB error
		dbErrorMsg := locales.GetMessage(localizer, "MsgReviewActionApprovedWithDBError", nil, nil)
		_ = m.answerCallbackQuery(ctx, queryID, dbErrorMsg, false)
		_ = m.deleteReviewMessage(ctx, adminID, reviewMessageID)
		_ = m.processNextSuggestion(ctx, adminID, session, index)
		return err // Return the error after attempting cleanup
	}

	approvedMsg := locales.GetMessage(localizer, "MsgReviewActionApproved", nil, nil)
	_ = m.answerCallbackQuery(ctx, queryID, approvedMsg, false)
	_ = m.deleteReviewMessage(ctx, adminID, reviewMessageID)
	return m.processNextSuggestion(ctx, adminID, session, index)
}

// handleRejectAction processes the rejection of a suggestion.
func (m *Manager) handleRejectAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, index int, reviewMessageID int) error {
	suggestion := session.Suggestions[index]

	// Create localizer (default to Russian)
	lang := locales.DefaultLanguage
	localizer := locales.NewLocalizer(lang)

	err := m.UpdateSuggestionStatus(ctx, suggestion.ID, models.StatusRejected, adminID)
	if err != nil {
		log.Printf("[handleRejectAction] Failed to update suggestion %s status to rejected: %v", suggestion.ID.Hex(), err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, queryID, errorMsg, true) // Use general error message
		// Don't proceed to next suggestion if DB update failed, admin might want to retry
		return err
	}

	rejectedMsg := locales.GetMessage(localizer, "MsgReviewActionRejected", nil, nil)
	_ = m.answerCallbackQuery(ctx, queryID, rejectedMsg, false)
	_ = m.deleteReviewMessage(ctx, adminID, reviewMessageID)
	return m.processNextSuggestion(ctx, adminID, session, index)
}

// handleNextAction processes the request to show the next suggestion (skip).
func (m *Manager) handleNextAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, currentIndex int) error {
	_ = m.answerCallbackQuery(ctx, queryID, "", false) // Acknowledge the button press

	// Skip the current suggestion by processing it as if it were completed
	return m.processNextSuggestion(ctx, adminID, session, currentIndex)
}

// publishSuggestion sends the approved suggestion to the target channel.
func (m *Manager) publishSuggestion(ctx context.Context, suggestion models.Suggestion) error {
	inputMedia := m.createInputMediaFromSuggestion(suggestion)
	if len(inputMedia) == 0 {
		return fmt.Errorf("no valid media found to publish for suggestion %s", suggestion.ID.Hex())
	}

	log.Printf("[publishSuggestion] Publishing suggestion %s to channel %d...", suggestion.ID.Hex(), m.targetChannelID)
	_, err := m.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
		ChatID: tu.ID(m.targetChannelID),
		Media:  inputMedia,
	})

	if err != nil {
		log.Printf("[publishSuggestion] Error sending media group for suggestion %s: %v", suggestion.ID.Hex(), err)
		return fmt.Errorf("failed to send media group to channel: %w", err)
	}

	log.Printf("[publishSuggestion] Successfully published suggestion %s", suggestion.ID.Hex())
	return nil
}

// processNextSuggestion removes the suggestion at the given index from the session
// and either sends the next one or cleans up the session.
func (m *Manager) processNextSuggestion(ctx context.Context, adminID int64, session *ReviewSession, completedIndex int) error {
	m.reviewSessionsMutex.Lock()
	defer m.reviewSessionsMutex.Unlock()

	session, ok := m.reviewSessions[adminID]
	if !ok {
		log.Printf("[processNextSuggestion] Session for admin %d disappeared.", adminID)
		return nil
	}

	if completedIndex < 0 || completedIndex >= len(session.Suggestions) {
		log.Printf("[processNextSuggestion] Invalid completedIndex %d for admin %d session (len %d).", completedIndex, adminID, len(session.Suggestions))
		delete(m.reviewSessions, adminID)
		return fmt.Errorf("invalid index %d in processNextSuggestion (session length %d)", completedIndex, len(session.Suggestions))
	}
	session.Suggestions = append(session.Suggestions[:completedIndex], session.Suggestions[completedIndex+1:]...)

	if len(session.Suggestions) > 0 {
		// The next suggestion to show is now at the 'completedIndex' position, if it exists.
		// If the removed item was the last one, this index is now out of bounds.
		nextIndex := completedIndex
		if nextIndex >= len(session.Suggestions) {
			nextIndex = len(session.Suggestions) - 1 // Show the new last item
		}

		m.reviewSessionsMutex.Unlock() // Release lock before potentially blocking call
		// Use session.ReviewChatID as the target chat ID
		chatIDErr := m.SendReviewMessage(ctx, session.ReviewChatID, adminID, nextIndex)
		m.reviewSessionsMutex.Lock() // Re-acquire lock
		return chatIDErr
	} else {
		log.Printf("[processNextSuggestion] Review batch finished for admin %d.", adminID)
		delete(m.reviewSessions, adminID)

		m.reviewSessionsMutex.Unlock()
		// Create localizer (default to Russian)
		lang := locales.DefaultLanguage
		// TODO: Get admin lang pref?
		localizer := locales.NewLocalizer(lang)
		queueEmptyMsg := locales.GetMessage(localizer, "MsgReviewQueueIsEmpty", nil, nil)
		// Use session.ReviewChatID as the target chat ID
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(session.ReviewChatID), queueEmptyMsg))
		m.reviewSessionsMutex.Lock() // Re-acquire lock
		if err != nil {
			log.Printf("[processNextSuggestion] Error sending queue empty message to chat %d (admin %d): %v", session.ReviewChatID, adminID, err)
		}
		return nil
	}
}
