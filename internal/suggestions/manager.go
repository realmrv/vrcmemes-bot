package suggestions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
	"vrcmemes-bot/internal/auth"
	"vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/database/models"
	"vrcmemes-bot/internal/locales"
	"vrcmemes-bot/internal/mediagroups"
	telegoapi "vrcmemes-bot/pkg/telegoapi"

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

	bot             telegoapi.BotAPI
	targetChannelID int64
	repo            database.SuggestionRepository
	adminChecker    auth.AdminCheckerInterface

	adminCache      []telego.ChatMember
	adminCacheTime  time.Time
	adminCacheMutex sync.RWMutex
	adminCacheTTL   time.Duration

	reviewSessions      map[int64]*ReviewSession
	reviewSessionsMutex sync.RWMutex

	feedbackRepo database.FeedbackRepository

	// Universal media group manager
	mediaGroupMgr *mediagroups.Manager
}

// NewManager creates a new suggestion manager.
func NewManager(
	bot telegoapi.BotAPI,
	repo database.SuggestionRepository,
	targetChannelID int64,
	adminChecker auth.AdminCheckerInterface,
	feedbackRepo database.FeedbackRepository,
	mediaGroupMgr *mediagroups.Manager,
) *Manager {
	if bot == nil {
		log.Fatal("Suggestion Manager: BotAPI instance is nil")
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
	if mediaGroupMgr == nil {
		log.Fatal("Suggestion Manager: Media group manager is nil")
	}

	return &Manager{
		userStates:      make(map[int64]UserState),
		bot:             bot,
		targetChannelID: targetChannelID,
		repo:            repo,
		feedbackRepo:    feedbackRepo,
		adminChecker:    adminChecker,
		mediaGroupMgr:   mediaGroupMgr,
		adminCache:      make([]telego.ChatMember, 0),
		adminCacheTTL:   5 * time.Minute,
		reviewSessions:  make(map[int64]*ReviewSession),
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
	lang := locales.GetDefaultLanguageTag().String()
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
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String())

	// Handle Media Group for Suggestion
	if message.MediaGroupID != "" {
		// Only handle photos in suggestion media groups for now
		if len(message.Photo) == 0 {
			log.Printf("[HandleSuggestionContent] Received non-photo message part of media group %s from user %d. Ignoring.", message.MediaGroupID, userID)
			return true, nil // Processed (ignored), state remains awaiting
		}

		// Delegate to universal manager
		err = m.mediaGroupMgr.HandleMessage(
			ctx, // Pass parent context
			*message,
			m.processSuggestionMediaGroup,   // Handler function
			mediagroups.DefaultProcessDelay, // Use default delay
			mediagroups.DefaultMaxGroupSize, // Use default size
		)
		if err != nil {
			log.Printf("[HandleSuggestionContent] Error delegating media group %s to manager: %v", message.MediaGroupID, err)
			// Handle error appropriately, maybe send message to user?
		}
		return true, err // Indicate message was handled (or attempted) by media group logic
	}

	// If it wasn't a media group, handle Single Photo for Suggestion
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
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String())
	mediaGroupID := message.MediaGroupID

	// --- Handle Media Group for Feedback ---
	if mediaGroupID != "" {
		// Handle any media type for feedback groups (photo/video)
		if len(message.Photo) == 0 && message.Video == nil {
			log.Printf("[HandleFeedbackContent] Received non-media message part of media group %s from user %d. Ignoring.", mediaGroupID, userID)
			return true, nil // Processed (ignored), state remains awaiting
		}

		// Delegate to universal manager
		err = m.mediaGroupMgr.HandleMessage(
			ctx,
			*message,
			m.processFeedbackMediaGroup,
			mediagroups.DefaultProcessDelay,
			mediagroups.DefaultMaxGroupSize,
		)
		if err != nil {
			log.Printf("[HandleFeedbackContent] Error delegating feedback media group %s to manager: %v", mediaGroupID, err)
			// Handle error appropriately
		}
		return true, err // Indicate message was handled (or attempted) by media group logic
	}
	// --- End Media Group Handling ---

	// If it wasn't a media group, handle Single Message (Text, Photo, Video)
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

// HandleFeedbackCommand handles the /feedback command logic.
func (m *Manager) HandleFeedbackCommand(ctx context.Context, update telego.Update) error {
	if update.Message == nil || update.Message.From == nil {
		log.Println("HandleFeedbackCommand: Received update without Message or From field.")
		return fmt.Errorf("invalid update received for feedback command")
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String())

	// Set user state to await feedback content
	m.SetUserState(userID, StateAwaitingFeedback)

	// Send prompt to user
	promptMsg := locales.GetMessage(localizer, "MsgFeedbackPrompt", nil, nil)
	_, err := m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), promptMsg))
	if err != nil {
		// Rollback state if prompt fails
		m.SetUserState(userID, StateIdle)
		log.Printf("[HandleFeedbackCommand User:%d] Error sending prompt: %v", userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		return fmt.Errorf("failed to send feedback prompt: %w", err)
	}

	log.Printf("[HandleFeedbackCommand User:%d] State set to StateAwaitingFeedback", userID)
	return nil
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
	memberPtr, err := m.bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: telego.ChatID{ID: m.targetChannelID},
		UserID: userID,
	})
	if err != nil {
		log.Printf("Error getting chat member %d from channel %d: %v", userID, m.targetChannelID, err)
		return false, fmt.Errorf("failed to get chat member info: %w", err)
	}

	if memberPtr == nil {
		log.Printf("GetChatMember returned nil interface for user %d in channel %d", userID, m.targetChannelID)
		return false, fmt.Errorf("failed to get chat member info (nil interface)")
	}

	// Get status using type assertion on the interface value
	var status string
	switch t := memberPtr.(type) {
	case *telego.ChatMemberOwner:
		status = t.Status
	case *telego.ChatMemberAdministrator:
		status = t.Status
	case *telego.ChatMemberMember:
		status = t.Status
	case *telego.ChatMemberRestricted:
		status = t.Status // Might still be considered a member depending on rules
	case *telego.ChatMemberLeft:
		status = t.Status // User has left
	case *telego.ChatMemberBanned:
		status = t.Status // User is banned
	default:
		log.Printf("Unknown chat member type for user %d in channel %d: %T", userID, m.targetChannelID, t)
		return false, fmt.Errorf("unknown chat member type")
	}

	// Check against allowed statuses
	allowedStatuses := []string{
		telego.MemberStatusCreator,
		telego.MemberStatusAdministrator,
		telego.MemberStatusMember,
		// telego.MemberStatusRestricted, // Decide if restricted users count
	}
	for _, allowed := range allowedStatuses {
		if status == allowed {
			return true, nil
		}
	}

	log.Printf("User %d has status '%s' in channel %d, which is not sufficient for subscription.", userID, status, m.targetChannelID)
	return false, nil // Not subscribed or has insufficient status (left, banned, restricted?)
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
func (m *Manager) UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status models.SuggestionStatus, reviewerID int64, adminUsername string) error {
	err := m.repo.UpdateSuggestionStatus(ctx, id, string(status), reviewerID, adminUsername)
	if err != nil {
		log.Printf("Error updating suggestion status in DB for ID %s: %v", id.Hex(), err)
		return err
	}
	log.Printf("Updated suggestion status in DB for ID %s to %s by reviewer %d (%s)", id.Hex(), status, reviewerID, adminUsername)
	return nil
}

// DeleteSuggestion removes a suggestion from the database.
func (m *Manager) DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error {
	return m.repo.DeleteSuggestion(ctx, id)
}

// processSuggestionMediaGroup is the handler function for suggestion media groups.
// Matches the mediagroups.ProcessFunc signature.
func (m *Manager) processSuggestionMediaGroup(ctx context.Context, groupID string, msgs []telego.Message) error {
	if len(msgs) == 0 {
		log.Printf("[ProcessSuggestionMediaGroup Group:%s] Group is empty.", groupID)
		return nil // Or return error?
	}
	firstMessage := msgs[0]
	userID := firstMessage.From.ID
	chatID := firstMessage.Chat.ID
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String())

	log.Printf("[ProcessSuggestionMediaGroup Group:%s User:%d] Processing %d messages.", groupID, userID, len(msgs))

	fileIDs := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Photo != nil && len(msg.Photo) > 0 {
			fileIDs = append(fileIDs, msg.Photo[len(msg.Photo)-1].FileID)
		}
	}

	if len(fileIDs) == 0 {
		log.Printf("[ProcessSuggestionMediaGroup Group:%s User:%d] No valid photos found in media group.", groupID, userID)
		m.SetUserState(userID, StateIdle) // Reset state
		// Send error message?
		return fmt.Errorf("no valid photos found in suggestion media group %s", groupID)
	}

	// Use caption from the first message if available
	caption := firstMessage.Caption

	suggestionForDB := &models.Suggestion{
		SuggesterID: userID,
		Username:    firstMessage.From.Username,
		FirstName:   firstMessage.From.FirstName,
		MessageID:   firstMessage.MessageID, // Use first message ID for reference
		ChatID:      chatID,
		FileIDs:     fileIDs,
		Caption:     caption,
		Status:      string(StatusPending),
		SubmittedAt: time.Now(),
	}

	err := m.AddSuggestion(ctx, suggestionForDB)
	if err != nil {
		log.Printf("[ProcessSuggestionMediaGroup Group:%s User:%d] Error saving suggestion: %v", groupID, userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgSuggestInternalProcessingError", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		m.SetUserState(userID, StateIdle) // Reset state on error
		return fmt.Errorf("failed to add suggestion for group %s: %w", groupID, err)
	}

	m.SetUserState(userID, StateIdle) // Reset state after successful processing
	confirmationMsg := locales.GetMessage(localizer, "MsgSuggestionReceivedConfirmation", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
	if err != nil {
		log.Printf("[ProcessSuggestionMediaGroup Group:%s User:%d] Error sending confirmation: %v", groupID, userID, err)
		// Don't return error here, main operation succeeded
	}
	return nil // Success
}

// processFeedbackMediaGroup is the handler function for feedback media groups.
// Matches the mediagroups.ProcessFunc signature.
func (m *Manager) processFeedbackMediaGroup(ctx context.Context, groupID string, msgs []telego.Message) error {
	if len(msgs) == 0 {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s] Group is empty.", groupID)
		return nil
	}
	firstMessage := msgs[0]
	userID := firstMessage.From.ID
	chatID := firstMessage.Chat.ID
	localizer := locales.NewLocalizer(locales.GetDefaultLanguageTag().String())

	log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Processing %d messages.", groupID, userID, len(msgs))

	photoIDs := make([]string, 0, len(msgs))
	videoIDs := make([]string, 0, len(msgs))
	feedbackText := firstMessage.Caption // Use caption from the first message

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

	// Text-only feedback is handled by handleFeedbackContent, media group MUST have media.
	if len(photoIDs) == 0 && len(videoIDs) == 0 {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] No valid media found in feedback group.", groupID, userID)
		m.SetUserState(userID, StateIdle) // Reset state
		return fmt.Errorf("no valid media found in feedback media group %s", groupID)
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
		MessageID:      firstMessage.MessageID,
	}

	err := m.feedbackRepo.AddFeedback(ctx, feedbackForDB)
	if err != nil {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Error saving feedback: %v", groupID, userID, err)
		errorMsg := locales.GetMessage(localizer, "MsgErrorGeneral", nil, nil)
		_, _ = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), errorMsg))
		m.SetUserState(userID, StateIdle) // Reset state on error
		return fmt.Errorf("failed to add feedback for group %s: %w", groupID, err)
	}

	m.SetUserState(userID, StateIdle) // Reset state after successful processing
	confirmationMsg := locales.GetMessage(localizer, "MsgFeedbackReceivedConfirmation", nil, nil)
	_, err = m.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), confirmationMsg))
	if err != nil {
		log.Printf("[ProcessFeedbackMediaGroup Group:%s User:%d] Error sending confirmation: %v", groupID, userID, err)
	}
	return nil // Success
}

// Add HandleCombinedMediaGroup to satisfy the interface defined in handlers/interfaces.go
func (m *Manager) HandleCombinedMediaGroup(ctx context.Context, groupID string, messages []telego.Message) error {
	if len(messages) == 0 {
		return errors.New("received empty media group for suggestion/feedback")
	}

	firstMessage := messages[0]
	userID := firstMessage.From.ID
	currentState := m.GetUserState(userID)

	log.Printf("[Manager.HandleCombinedMediaGroup Group:%s] User %d State %v", groupID, userID, currentState)

	switch currentState {
	case StateAwaitingSuggestion:
		return m.processSuggestionMediaGroup(ctx, groupID, messages)
	case StateAwaitingFeedback:
		return m.processFeedbackMediaGroup(ctx, groupID, messages)
	default:
		log.Printf("[Manager.HandleCombinedMediaGroup Group:%s] User %d state %v is not awaiting suggestion or feedback. Ignoring.", groupID, userID, currentState)
		return nil // Not an error, just not handled here
	}
}
