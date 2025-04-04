package suggestions

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"vrcmemes-bot/pkg/locales"
)

// HandleCallbackQuery handles callback queries for suggestion review.
// Returns true if the callback was processed by this handler, false otherwise.
func (m *Manager) HandleCallbackQuery(ctx context.Context, query telego.CallbackQuery) (processed bool, err error) {
	adminID := query.From.ID
	callbackData := query.Data

	if !strings.HasPrefix(callbackData, "review:") {
		return false, nil
	}

	// Parse callback data
	parts := strings.Split(query.Data, ":")

	lang := locales.DefaultLanguage
	if query.From.LanguageCode != "" {
		// lang = query.From.LanguageCode // TODO: Use user language
	}
	localizer := locales.NewLocalizer(lang)

	if len(parts) != 4 {
		log.Printf("[CallbackQuery] Invalid data format: %s", callbackData)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, errorMsg, true)
		return true, fmt.Errorf("invalid callback data format")
	}

	suggestionIDHex := parts[1]
	action := parts[2]
	indexStr := parts[3]
	currentIndex, err := strconv.Atoi(indexStr)
	if err != nil {
		log.Printf("[CallbackQuery] Invalid index in data: %s, err: %v", callbackData, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, errorMsg, true)
		return true, fmt.Errorf("invalid index in callback data")
	}

	suggestionID, err := primitive.ObjectIDFromHex(suggestionIDHex)
	if err != nil {
		log.Printf("[CallbackQuery] Invalid suggestion ID hex in data: %s, err: %v", callbackData, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, errorMsg, true)
		return true, fmt.Errorf("invalid suggestion ID in callback data")
	}

	log.Printf("[CallbackQuery] Parsed: Admin=%d, SugID=%s, Action=%s, Index=%d", adminID, suggestionIDHex, action, currentIndex)

	isAdmin, err := m.IsAdmin(ctx, adminID)
	if err != nil {
		log.Printf("[CallbackQuery] Error checking admin status for user %d: %v", adminID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, errorMsg, true)
		return true, err
	}
	if !isAdmin {
		log.Printf("[CallbackQuery] User %d is not admin, ignoring review action.", adminID)
		adminErrorMsg := locales.GetMessage(localizer, "MsgErrorRequiresAdmin", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, adminErrorMsg, true)
		return true, nil
	}

	m.reviewSessionsMutex.RLock()
	session, sessionExists := m.reviewSessions[adminID]
	m.reviewSessionsMutex.RUnlock()

	if !sessionExists || currentIndex < 0 || currentIndex >= len(session.Suggestions) || session.Suggestions[currentIndex].ID != suggestionID {
		log.Printf("[CallbackQuery] Invalid session or suggestion mismatch for admin %d, index %d, ID %s", adminID, currentIndex, suggestionIDHex)
		expiredMsg := locales.GetMessage(localizer, "MsgReviewSessionExpired", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, expiredMsg, true)
		m.reviewSessionsMutex.Lock()
		delete(m.reviewSessions, adminID)
		m.reviewSessionsMutex.Unlock()
		return true, nil
	}

	var originalReviewMessageID int
	if query.Message != nil {
		if msg, ok := query.Message.(*telego.Message); ok && msg != nil {
			originalReviewMessageID = msg.MessageID
		} else {
			log.Printf("[CallbackQuery] Warning: Could not get MessageID from callback query message for admin %d", adminID)
		}
	}

	switch action {
	case "approve":
		log.Printf("[CallbackQuery] Action: Approve for SugID %s by Admin %d", suggestionIDHex, adminID)
		err := m.handleApproveAction(ctx, query.ID, adminID, session, currentIndex, originalReviewMessageID)
		if err != nil {
			log.Printf("[CallbackQuery] Error handling approve action: %v", err)
			return true, err
		}
	case "reject":
		log.Printf("[CallbackQuery] Action: Reject for SugID %s by Admin %d", suggestionIDHex, adminID)
		err := m.handleRejectAction(ctx, query.ID, adminID, session, currentIndex, originalReviewMessageID)
		if err != nil {
			log.Printf("[CallbackQuery] Error handling reject action: %v", err)
			return true, err
		}
	case "next":
		log.Printf("[CallbackQuery] Action: Next for SugID %s by Admin %d", suggestionIDHex, adminID)
		err := m.handleNextAction(ctx, query.ID, adminID, session, currentIndex)
		if err != nil {
			log.Printf("[CallbackQuery] Error handling next action: %v", err)
			return true, err
		}
	case "previous":
		log.Printf("[CallbackQuery] Action: Previous for SugID %s by Admin %d", suggestionIDHex, adminID)
		err := m.handlePreviousAction(ctx, query.ID, adminID, session, currentIndex)
		if err != nil {
			log.Printf("[CallbackQuery] Error handling previous action: %v", err)
			return true, err
		}
	default:
		log.Printf("[CallbackQuery] Unknown action: %s", action)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_ = m.answerCallbackQuery(ctx, query.ID, errorMsg, true)
		return true, fmt.Errorf("unknown review action: %s", action)
	}

	return true, nil
}
