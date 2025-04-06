package suggestions

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
	"vrcmemes-bot/internal/auth"
	"vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	// Define constants used in this file
	mediaGroupProcessDelay = 3 * time.Second // Delay for processing suggestion/feedback media groups
	maxMediaGroupSize      = 10              // Max items in suggestion/feedback media groups
)

// Manager handles the suggestion logic and storage.
type Manager struct {
	userStates   map[int64]UserState
	muUserStates sync.RWMutex

	suggestionMediaGroups   map[string][]telego.Message
	muSuggestionMediaGroups sync.Mutex

	bot             *telego.Bot
	targetChannelID int64
	repo            database.SuggestionRepository
	adminChecker    *auth.AdminChecker

	adminCache      []telego.ChatMember
	adminCacheTime  time.Time
	adminCacheMutex sync.RWMutex
	adminCacheTTL   time.Duration

	reviewSessions      map[int64]*ReviewSession
	reviewSessionsMutex sync.RWMutex

	// feedbackRepo is needed to save feedback in handleFeedbackContent
	feedbackRepo database.FeedbackRepository

	// Temporary storage for feedback media groups
	feedbackMediaGroups   map[string][]telego.Message
	muFeedbackMediaGroups sync.Mutex
}

// NewManager creates a new suggestion manager.
func NewManager(bot *telego.Bot, repo database.SuggestionRepository, targetChannelID int64, adminChecker *auth.AdminChecker, feedbackRepo database.FeedbackRepository) *Manager {
	if bot == nil {
		log.Fatal("Suggestion Manager: Telego bot instance is nil")
	}
	if repo == nil {
		log.Fatal("Suggestion Manager: Suggestion repository is nil")
	}
	if feedbackRepo == nil {
		log.Fatal("Suggestion Manager: Feedback repository is nil")
	}
	if targetChannelID == 0 {
		log.Fatal("Suggestion Manager: Target channel ID is not set")
	}
	if adminChecker == nil {
		log.Fatal("Suggestion Manager: Admin checker is nil")
	}

	return &Manager{
		userStates:            make(map[int64]UserState),
		suggestionMediaGroups: make(map[string][]telego.Message),
		feedbackMediaGroups:   make(map[string][]telego.Message),
		bot:                   bot,
		targetChannelID:       targetChannelID,
		repo:                  repo,
		feedbackRepo:          feedbackRepo, // Store feedback repo
		adminChecker:          adminChecker,
		adminCache:            make([]telego.ChatMember, 0),
		adminCacheTTL:         5 * time.Minute,
		reviewSessions:        make(map[int64]*ReviewSession),
	}
}

// HandleSuggestCommand handles the /suggest command.
func (m *Manager) HandleSuggestCommand(ctx context.Context, update telego.Update) error {
	if update.Message == nil || update.Message.From == nil {
		log.Println("HandleSuggestCommand: Received update without Message or From field.")
		return fmt.Errorf("invalid update received for suggest command")
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	// Default to Russian
	lang := locales.DefaultLanguage
	localizer := locales.NewLocalizer(lang)

	if m.GetUserState(userID) == StateAwaitingSuggestion {
		msg := locales.GetMessage(localizer, "MsgSuggestAlreadyWaitingForContent", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return err
	}

	isSubscribed, err := m.CheckSubscription(ctx, userID)
	if err != nil {
		log.Printf("Error checking subscription for user %d: %v", userID, err)
		msg := locales.GetMessage(localizer, "MsgSuggestErrorCheckingSubscription", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		if sendErr != nil {
			log.Printf("Error sending subscription check error message to user %d: %v", userID, sendErr)
		}
		return err
	}

	if !isSubscribed {
		msg := locales.GetMessage(localizer, "MsgSuggestRequiresSubscription", nil, nil)
		_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))
		return err
	}

	m.SetUserState(userID, StateAwaitingSuggestion)
	promptMsg := locales.GetMessage(localizer, "MsgSuggestSendContentPrompt", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), promptMsg))
	if err != nil {
		m.SetUserState(userID, StateIdle) // Rollback state if sending prompt fails
		log.Printf("Error sending suggest prompt to user %d: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
		_, sendErr := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		if sendErr != nil {
			log.Printf("Error sending internal error message after prompt failure to user %d: %v", userID, sendErr)
		}
		return err
	}

	return nil
}

// HandleMessage processes incoming messages potentially containing suggestion content.
func (m *Manager) HandleMessage(ctx context.Context, update telego.Update) (processed bool, err error) {
	if update.Message == nil || update.Message.From == nil {
		return false, nil // Not a message we can process here
	}

	userID := update.Message.From.ID
	currentState := m.GetUserState(userID)
	log.Printf("[Suggest Manager HandleMessage User:%d] Current state: %v", userID, currentState)

	switch currentState {
	case StateAwaitingSuggestion:
		log.Printf("[Suggest Manager HandleMessage User:%d] Handling as suggestion...", userID)
		return m.handleSuggestionContent(ctx, update.Message)
	case StateAwaitingFeedback:
		log.Printf("[Suggest Manager HandleMessage User:%d] Handling as feedback...", userID)
		// Pass the feedback repository needed by handleFeedbackContent
		return m.handleFeedbackContent(ctx, update.Message)
	default:
		log.Printf("[Suggest Manager HandleMessage User:%d] State is not AwaitingSuggestion or AwaitingFeedback, returning processed=false", userID)
		return false, nil
	}
}

// handleSuggestionContent handles a message when the user is in StateAwaitingSuggestion.
func (m *Manager) handleSuggestionContent(ctx context.Context, message *telego.Message) (processed bool, err error) {
	userID := message.From.ID
	chatID := message.Chat.ID
	localizer := locales.NewLocalizer(locales.DefaultLanguage) // Assuming default lang for now

	// Handle Media Group for Suggestion
	if message.MediaGroupID != "" {
		// Only handle photos in suggestion media groups for now
		if len(message.Photo) == 0 {
			log.Printf("[HandleSuggestionContent] Received non-photo message part of media group %s from user %d. Ignoring.", message.MediaGroupID, userID)
			return true, nil // Processed (ignored), state remains awaiting
		}

		m.addMessageToSuggestionGroup(*message)
		group := m.getSuggestionGroup(message.MediaGroupID)

		if len(group) == 1 {
			log.Printf("[HandleSuggestionContent] Started suggestion media group timer for group %s, user %d", message.MediaGroupID, userID)
			msg := locales.GetMessage(localizer, "MsgSuggestionMediaGroupPartReceived", nil, nil)
			_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))

			// Use background context for timer function
			bgCtx := context.Background()
			time.AfterFunc(mediaGroupProcessDelay, func() {
				m.processSuggestionMediaGroup(bgCtx, userID, chatID, message.MediaGroupID)
			})
		}
		return true, nil // Processed, state remains awaiting until timer fires
	}

	// Handle Single Photo for Suggestion
	if message.Photo != nil && len(message.Photo) > 0 {
		fileIDs := []string{message.Photo[len(message.Photo)-1].FileID}
		caption := message.Caption // User-provided caption for admin review

		suggestionForDB := &models.Suggestion{
			SuggesterID: userID,
			Username:    message.From.Username,
			FirstName:   message.From.FirstName,
			MessageID:   message.MessageID,
			ChatID:      chatID,
			FileIDs:     fileIDs,
			Caption:     caption,
			Status:      string(StatusPending),
			SubmittedAt: time.Now(),
		}
		err = m.AddSuggestion(ctx, suggestionForDB)
		if err != nil {
			log.Printf("[HandleSuggestionContent] Error saving single photo suggestion for user %d: %v", userID, err)
			errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
			_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
			m.SetUserState(userID, StateIdle) // Reset state on error
			return true, err                  // Processed (with error)
		}

		m.SetUserState(userID, StateIdle) // Reset state after success
		confirmationMsg := locales.GetMessage(localizer, "MsgSuggestionReceivedConfirmation", nil, nil)
		_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
		if err != nil {
			log.Printf("[HandleSuggestionContent] Error sending single photo confirmation to user %d: %v", userID, err)
		}
		return true, nil // Processed successfully
	}

	// Wrong Message Type for Suggestion
	log.Printf("[HandleSuggestionContent] User %d sent non-photo/non-media-group message while awaiting suggestion.", userID)
	errorMsg := locales.GetMessage(localizer, "MsgSuggestionRequiresPhoto", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
	// Do not reset state here, let the user try again
	return true, err // Processed (with error message sent)
}

// extractFeedbackContent extracts text, photo/video IDs from a single feedback message.
func extractFeedbackContent(message *telego.Message) (text string, photoIDs []string, videoIDs []string) {
	photoIDs = []string{}
	videoIDs = []string{}
	text = ""

	if message.Text != "" {
		text = message.Text
	}

	// Extract photo
	if message.Photo != nil && len(message.Photo) > 0 {
		bestPhoto := message.Photo[0]
		for _, p := range message.Photo {
			if p.FileSize > bestPhoto.FileSize {
				bestPhoto = p
			}
		}
		photoIDs = append(photoIDs, bestPhoto.FileID)
		if text == "" && message.Caption != "" { // Use caption as text if text is empty
			text = message.Caption
		}
	}

	// Extract video
	if message.Video != nil {
		videoIDs = append(videoIDs, message.Video.FileID)
		if text == "" && message.Caption != "" { // Use caption as text if text is empty
			text = message.Caption
		}
	}
	return text, photoIDs, videoIDs
}

// handleFeedbackContent handles a message when the user is in StateAwaitingFeedback.
// Refactored to extract content parsing.
func (m *Manager) handleFeedbackContent(ctx context.Context, message *telego.Message) (processed bool, err error) {
	userID := message.From.ID
	chatID := message.Chat.ID
	localizer := locales.NewLocalizer(locales.DefaultLanguage) // Assuming default lang for now
	mediaGroupID := message.MediaGroupID

	// --- Handle Media Group for Feedback ---
	if mediaGroupID != "" {
		msgs := m.addMessageToFeedbackGroup(*message) // Assuming addMessageToFeedbackGroup exists/is added
		if len(msgs) == 1 {
			// Start timer only on the first message of the group
			log.Printf("[HandleFeedbackContent] Started feedback media group timer for group %s, user %d", mediaGroupID, userID)
			msg := locales.GetMessage(localizer, "MsgFeedbackMediaGroupPartReceived", nil, nil)
			_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), msg))

			bgCtx := context.Background()
			time.AfterFunc(mediaGroupProcessDelay, func() {
				m.processFeedbackMediaGroup(bgCtx, userID, chatID, mediaGroupID)
			})
		}
		return true, nil // Media group message stored, wait for timer
	}
	// --- End Media Group Handling ---

	// --- Extract Content from Single Message ---
	feedbackText, photoIDs, videoIDs := extractFeedbackContent(message)

	// --- Validate Content ---
	if feedbackText == "" && len(photoIDs) == 0 && len(videoIDs) == 0 {
		log.Printf("[HandleFeedbackContent] User %d sent empty message while awaiting feedback.", userID)
		errorMsg := locales.GetMessage(localizer, "MsgFeedbackRequiresContent", nil, nil)
		_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		// Don't reset state, let them try again
		return true, err // Processed (with error sent)
	}

	// --- Save Feedback ---
	feedbackForDB := &models.Feedback{
		UserID:         userID,
		Username:       message.From.Username,
		FirstName:      message.From.FirstName,
		Text:           feedbackText,
		PhotoIDs:       photoIDs,
		VideoIDs:       videoIDs,
		MediaGroupID:   "", // Empty for single messages
		OriginalChatID: chatID,
		MessageID:      message.MessageID,
		// SubmittedAt will be set by the repository
	}

	err = m.feedbackRepo.AddFeedback(ctx, feedbackForDB)
	if err != nil {
		log.Printf("[HandleFeedbackContent] Error saving feedback for user %d: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		m.SetUserState(userID, StateIdle) // Reset state on error
		return true, err                  // Processed (with error)
	}

	// --- Confirm and Reset State ---
	m.SetUserState(userID, StateIdle) // Reset state after successful submission
	confirmationMsg := locales.GetMessage(localizer, "MsgFeedbackReceivedConfirmation", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
	if err != nil {
		log.Printf("[HandleFeedbackContent] Error sending feedback confirmation to user %d: %v", userID, err)
	}
	return true, nil // Processed successfully
}

// SetUserState sets the state for a given user.
func (m *Manager) SetUserState(userID int64, state UserState) {
	m.muUserStates.Lock()
	defer m.muUserStates.Unlock()
	if state == StateIdle {
		delete(m.userStates, userID)
	} else {
		m.userStates[userID] = state
	}
}

// GetUserState retrieves the state for a given user.
func (m *Manager) GetUserState(userID int64) UserState {
	m.muUserStates.RLock()
	defer m.muUserStates.RUnlock()
	return m.userStates[userID]
}

// CheckSubscription checks if a user is a member of the target channel.
func (m *Manager) CheckSubscription(ctx context.Context, userID int64) (bool, error) {
	member, err := m.bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: telego.ChatID{ID: m.targetChannelID},
		UserID: userID,
	})
	if err != nil {
		// Consider specific API errors if needed
		log.Printf("Error getting chat member %d from channel %d: %v", userID, m.targetChannelID, err)
		return false, fmt.Errorf("failed to get chat member info: %w", err)
	}

	allowedStatuses := []string{"creator", "administrator", "member", "restricted"}
	status := member.MemberStatus()
	for _, allowed := range allowedStatuses {
		if status == allowed {
			return true, nil
		}
	}
	return false, nil
}

// AddSuggestion saves a new suggestion to the database.
func (m *Manager) AddSuggestion(ctx context.Context, suggestion *models.Suggestion) error {
	suggestion.Status = string(StatusPending)
	err := m.repo.CreateSuggestion(ctx, suggestion)
	if err != nil {
		log.Printf("Error creating suggestion in DB for user %d: %v", suggestion.SuggesterID, err)
		return err
	}
	log.Printf("Created suggestion in DB with ID %s from user %d", suggestion.ID.Hex(), suggestion.SuggesterID)
	return nil
}

// GetPendingSuggestions retrieves pending suggestions from the database.
func (m *Manager) GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error) {
	return m.repo.GetPendingSuggestions(ctx, limit, offset)
}

// GetSuggestionByID retrieves a suggestion by its MongoDB ObjectID.
func (m *Manager) GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error) {
	return m.repo.GetSuggestionByID(ctx, id)
}

// UpdateSuggestionStatus updates the status of a suggestion in the database.
func (m *Manager) UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status models.SuggestionStatus, reviewerID int64) error {
	err := m.repo.UpdateSuggestionStatus(ctx, id, string(status), reviewerID)
	if err != nil {
		log.Printf("Error updating suggestion status in DB for ID %s: %v", id.Hex(), err)
		return err
	}
	log.Printf("Updated suggestion status in DB for ID %s to %s by reviewer %d", id.Hex(), status, reviewerID)
	return nil
}

// DeleteSuggestion removes a suggestion from the database.
func (m *Manager) DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error {
	// This might not be needed directly if suggestions are marked, but implemented for completeness
	return m.repo.DeleteSuggestion(ctx, id)
}

// ProcessSuggestionCallback satisfies the database.SuggestionManager interface.
// It delegates the call to the internal HandleCallbackQuery method.
func (m *Manager) ProcessSuggestionCallback(ctx context.Context, query telego.CallbackQuery) (processed bool, err error) {
	// Note: HandleCallbackQuery might need adjustments if its signature or behavior
	// doesn't perfectly match the interface requirements (e.g., error handling).
	return m.HandleCallbackQuery(ctx, query)
}

// --- Helper functions for Feedback Media Groups (similar to suggestion ones) ---

// addMessageToFeedbackGroup adds a message to the temporary storage for feedback media groups.
func (m *Manager) addMessageToFeedbackGroup(message telego.Message) []telego.Message {
	m.muFeedbackMediaGroups.Lock()
	defer m.muFeedbackMediaGroups.Unlock()

	group, exists := m.feedbackMediaGroups[message.MediaGroupID]
	if !exists {
		group = make([]telego.Message, 0, 10) // Max 10 media items
	}

	// Avoid duplicates and limit size
	found := false
	for _, msg := range group {
		if msg.MessageID == message.MessageID {
			found = true
			break
		}
	}
	if !found && len(group) < maxMediaGroupSize { // Reuse constant
		group = append(group, message)
		// Sort by message ID to maintain order
		sort.Slice(group, func(i, j int) bool {
			return group[i].MessageID < group[j].MessageID
		})
		m.feedbackMediaGroups[message.MediaGroupID] = group
		log.Printf("[Feedback Media Group Store Group:%s] Added message %d. Total: %d", message.MediaGroupID, message.MessageID, len(group))
	} else if len(group) >= maxMediaGroupSize {
		log.Printf("[Feedback Media Group Store Group:%s] Group limit reached, message %d dropped.", message.MediaGroupID, message.MessageID)
	}

	return group
}

// getFeedbackGroup retrieves the collected messages for a feedback media group.
func (m *Manager) getFeedbackGroup(groupID string) []telego.Message {
	m.muFeedbackMediaGroups.Lock()
	defer m.muFeedbackMediaGroups.Unlock()
	// Return a copy to avoid race conditions if the caller modifies it
	group := m.feedbackMediaGroups[groupID]
	groupCopy := make([]telego.Message, len(group))
	copy(groupCopy, group)
	return groupCopy
}

// deleteFeedbackGroup removes a feedback media group from temporary storage.
func (m *Manager) deleteFeedbackGroup(groupID string) {
	m.muFeedbackMediaGroups.Lock()
	defer m.muFeedbackMediaGroups.Unlock()
	delete(m.feedbackMediaGroups, groupID)
	log.Printf("[Feedback Media Group Cleanup Group:%s] Deleted group from storage.", groupID)
}

// processFeedbackMediaGroup processes a collected feedback media group after the timer expires.
func (m *Manager) processFeedbackMediaGroup(ctx context.Context, userID, chatID int64, groupID string) {
	log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Timer fired. Processing group.", groupID, userID)
	msgs := m.getFeedbackGroup(groupID)
	m.deleteFeedbackGroup(groupID) // Clean up immediately

	if len(msgs) == 0 {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] No messages found for group after timer. Aborting.", groupID, userID)
		m.SetUserState(userID, StateIdle) // Reset state
		return
	}

	localizer := locales.NewLocalizer(locales.DefaultLanguage)
	photoIDs := []string{}
	videoIDs := []string{}
	feedbackText := "" // Caption from the first message is used

	// Extract file IDs and caption from the first message
	firstMessage := msgs[0]
	if firstMessage.Caption != "" {
		feedbackText = firstMessage.Caption
	}

	for _, msg := range msgs {
		if msg.Photo != nil && len(msg.Photo) > 0 {
			bestPhoto := msg.Photo[0]
			for _, p := range msg.Photo {
				if p.FileSize > bestPhoto.FileSize {
					bestPhoto = p
				}
			}
			photoIDs = append(photoIDs, bestPhoto.FileID)
		} else if msg.Video != nil {
			videoIDs = append(videoIDs, msg.Video.FileID)
		}
	}

	// Should not happen if we filtered earlier, but double-check
	if len(photoIDs) == 0 && len(videoIDs) == 0 {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] No valid media found in group. Aborting.", groupID, userID)
		m.SetUserState(userID, StateIdle) // Reset state
		return
	}

	feedbackForDB := &models.Feedback{
		UserID:         userID,
		Username:       firstMessage.From.Username,
		FirstName:      firstMessage.From.FirstName,
		Text:           feedbackText,
		PhotoIDs:       photoIDs,
		VideoIDs:       videoIDs,
		MediaGroupID:   groupID,
		SubmittedAt:    time.Now(),
		OriginalChatID: chatID,
		MessageID:      firstMessage.MessageID, // Use first message ID for reference
	}

	err := m.feedbackRepo.AddFeedback(ctx, feedbackForDB)
	if err != nil {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Error saving feedback: %v", groupID, userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		// State is already Idle if saving failed within handleFeedbackContent, but set again for clarity
		m.SetUserState(userID, StateIdle)
		return
	}

	m.SetUserState(userID, StateIdle) // Reset state after successful processing
	confirmationMsg := locales.GetMessage(localizer, "MsgFeedbackReceivedConfirmation", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
	if err != nil {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Error sending confirmation: %v", groupID, userID, err)
	}
}
