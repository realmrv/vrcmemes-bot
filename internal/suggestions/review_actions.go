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

// handleApproveAction approves a suggestion, posts it, cleans up messages, and proceeds.
func (m *Manager) handleApproveAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, index int, _ int /* reviewMessageID - no longer needed directly */) error {
	suggestion := session.Suggestions[index]
	localizer := locales.NewLocalizer(locales.DefaultLanguage) // TODO: Use admin lang pref

	// Approve and publish
	dbErr := m.UpdateSuggestionStatus(ctx, suggestion.ID, models.StatusApproved, adminID)
	publishErr := m.publishSuggestion(ctx, suggestion)

	// Determine response message
	var responseMsg string
	if publishErr != nil {
		log.Printf("[ApproveAction] Error publishing suggestion %s: %v", suggestion.ID.Hex(), publishErr)
		responseMsg = locales.GetMessage(localizer, "MsgReviewErrorDuringPublishing", nil, nil)
		if dbErr != nil {
			// Append DB error suffix if both failed
			responseMsg += locales.GetMessage(localizer, "MsgErrorDBUpdateFailedSuffix", nil, nil)
		}
	} else {
		responseMsg = locales.GetMessage(localizer, "MsgReviewActionApproved", nil, nil)
		if dbErr != nil {
			// Append DB error suffix if publishing succeeded but DB update failed
			responseMsg = locales.GetMessage(localizer, "MsgReviewActionApprovedWithDBError", nil, nil)
		}
	}

	// Answer callback query first
	_ = m.answerCallbackQuery(ctx, queryID, responseMsg, false)

	// Delete the original review messages (media + control)
	go m.deleteReviewMessages(context.Background(), session.ReviewChatID, session.CurrentMediaMessageIDs, session.CurrentControlMessageID)

	// Remove suggestion and send next or finish
	m.reviewSessionsMutex.Lock()
	currentSession, ok := m.reviewSessions[adminID]
	if !ok {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[ApproveAction Admin:%d] Session disappeared before removing suggestion.", adminID)
		return nil // Session gone, nothing more to do
	}
	if index < 0 || index >= len(currentSession.Suggestions) {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[ApproveAction Admin:%d] Invalid index %d for removal (len %d).", adminID, index, len(currentSession.Suggestions))
		return fmt.Errorf("invalid index %d during approve action", index)
	}
	currentSession.Suggestions = append(currentSession.Suggestions[:index], currentSession.Suggestions[index+1:]...)
	err := m.sendNextOrFinishReview(ctx, adminID, currentSession)
	m.reviewSessionsMutex.Unlock()
	return err
}

// handleRejectAction rejects a suggestion, cleans up messages, and proceeds.
func (m *Manager) handleRejectAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, index int, _ int /* reviewMessageID - no longer needed directly */) error {
	suggestion := session.Suggestions[index]
	localizer := locales.NewLocalizer(locales.DefaultLanguage) // TODO: Use admin lang pref

	// Reject suggestion in DB
	dbErr := m.UpdateSuggestionStatus(ctx, suggestion.ID, models.StatusRejected, adminID)

	// Determine response message
	responseMsg := locales.GetMessage(localizer, "MsgReviewActionRejected", nil, nil)
	if dbErr != nil {
		log.Printf("[RejectAction] Error updating suggestion %s status to rejected: %v", suggestion.ID.Hex(), dbErr)
		responseMsg += locales.GetMessage(localizer, "MsgErrorDBUpdateFailedSuffix", nil, nil)
	}

	// Answer callback query first
	_ = m.answerCallbackQuery(ctx, queryID, responseMsg, false)

	// Delete the original review messages (media + control)
	go m.deleteReviewMessages(context.Background(), session.ReviewChatID, session.CurrentMediaMessageIDs, session.CurrentControlMessageID)

	// Remove suggestion and send next or finish
	m.reviewSessionsMutex.Lock()
	currentSession, ok := m.reviewSessions[adminID]
	if !ok {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[RejectAction Admin:%d] Session disappeared before removing suggestion.", adminID)
		return nil // Session gone, nothing more to do
	}
	if index < 0 || index >= len(currentSession.Suggestions) {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[RejectAction Admin:%d] Invalid index %d for removal (len %d).", adminID, index, len(currentSession.Suggestions))
		return fmt.Errorf("invalid index %d during reject action", index)
	}
	currentSession.Suggestions = append(currentSession.Suggestions[:index], currentSession.Suggestions[index+1:]...)
	err := m.sendNextOrFinishReview(ctx, adminID, currentSession)
	m.reviewSessionsMutex.Unlock()
	return err
}

// handleNextAction moves to the next suggestion in the review session, cleaning up old messages.
func (m *Manager) handleNextAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, currentIndex int) error {
	// Answer callback query immediately (no text needed for "next")
	_ = m.answerCallbackQuery(ctx, queryID, "", false)

	// Delete the current review messages (media + control)
	go m.deleteReviewMessages(context.Background(), session.ReviewChatID, session.CurrentMediaMessageIDs, session.CurrentControlMessageID)

	// Send the next suggestion message
	nextIndex := currentIndex + 1
	if nextIndex >= len(session.Suggestions) {
		// Should ideally not happen if button wasn't shown, but handle defensively
		log.Printf("[NextAction Admin:%d] Attempted to go next from last index %d.", adminID, currentIndex)
		// Wrap around? Send finished message? Resending last for now.
		nextIndex = len(session.Suggestions) - 1
	}

	m.reviewSessionsMutex.Lock()
	session, ok := m.reviewSessions[adminID]
	if !ok {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[NextAction Admin:%d] Session disappeared before sending next message.", adminID)
		return nil
	}
	m.reviewSessionsMutex.Unlock()

	return m.SendReviewMessage(ctx, session.ReviewChatID, adminID, nextIndex)
}

// handlePreviousAction moves to the previous suggestion in the review session, cleaning up old messages.
func (m *Manager) handlePreviousAction(ctx context.Context, queryID string, adminID int64, session *ReviewSession, currentIndex int) error {
	// Answer callback query immediately
	_ = m.answerCallbackQuery(ctx, queryID, "", false)

	// Delete the current review messages (media + control)
	go m.deleteReviewMessages(context.Background(), session.ReviewChatID, session.CurrentMediaMessageIDs, session.CurrentControlMessageID)

	// Send the previous suggestion message
	previousIndex := currentIndex - 1
	if previousIndex < 0 {
		// Should ideally not happen if button wasn't shown, but handle defensively
		log.Printf("[PreviousAction Admin:%d] Attempted to go previous from index 0.", adminID)
		// Resend the first message? Or send error? Resending first for now.
		previousIndex = 0
	}

	m.reviewSessionsMutex.Lock() // Lock needed before accessing session again outside processNextSuggestion
	session, ok := m.reviewSessions[adminID]
	if !ok {
		m.reviewSessionsMutex.Unlock()
		log.Printf("[PreviousAction Admin:%d] Session disappeared before sending previous message.", adminID)
		return nil
	}
	m.reviewSessionsMutex.Unlock()

	return m.SendReviewMessage(ctx, session.ReviewChatID, adminID, previousIndex)
}

// sendNextOrFinishReview sends the next suggestion or finishes the session if the queue is empty.
// Assumes the lock is held by the caller if session modification occurred.
func (m *Manager) sendNextOrFinishReview(ctx context.Context, adminID int64, session *ReviewSession) error {
	if len(session.Suggestions) > 0 {
		// The next suggestion to show is now at index 0 after deletion
		nextIndex := 0
		log.Printf("[sendNextOrFinishReview Admin:%d] Sending next suggestion at index %d", adminID, nextIndex)
		m.reviewSessionsMutex.Unlock() // Release lock before blocking call
		chatIDErr := m.SendReviewMessage(ctx, session.ReviewChatID, adminID, nextIndex)
		m.reviewSessionsMutex.Lock() // Re-acquire lock
		return chatIDErr
	} else {
		// No more suggestions in this batch
		log.Printf("[sendNextOrFinishReview Admin:%d] Review batch finished.", adminID)
		delete(m.reviewSessions, adminID) // Delete the session

		m.reviewSessionsMutex.Unlock()
		lang := locales.DefaultLanguage
		localizer := locales.NewLocalizer(lang)
		queueEmptyMsg := locales.GetMessage(localizer, "MsgReviewQueueIsEmpty", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(session.ReviewChatID), queueEmptyMsg))
		m.reviewSessionsMutex.Lock() // Re-acquire lock
		if err != nil {
			log.Printf("[sendNextOrFinishReview Admin:%d] Error sending queue empty message: %v", adminID, err)
		}
		return nil // End of batch is not an error
	}
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

// processNextSuggestion (REMOVED/REPLACED by sendNextOrFinishReview)
/* func (m *Manager) processNextSuggestion(ctx context.Context, adminID int64, session *ReviewSession, completedIndex int) error { ... } */
