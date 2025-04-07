package handlers

import (
	"context"
	"errors"
	"testing"
	"time"
	"vrcmemes-bot/internal/database/models" // Add import for models
	"vrcmemes-bot/internal/locales"         // Add mediagroups import
	"vrcmemes-bot/internal/suggestions"

	// Import for BotAPI
	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// --- Mocks ---

// MockBot is a mock implementing the telegoapi.BotAPI interface
type MockBot struct {
	mock.Mock
}

func (m *MockBot) SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	args := m.Called(ctx, params)
	if msg, ok := args.Get(0).(*telego.Message); ok {
		return msg, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockBot) GetMe(ctx context.Context) (*telego.User, error) {
	args := m.Called(ctx)
	if user, ok := args.Get(0).(*telego.User); ok {
		return user, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockBot) CopyMessage(ctx context.Context, params *telego.CopyMessageParams) (*telego.MessageID, error) {
	args := m.Called(ctx, params)
	if msgID, ok := args.Get(0).(*telego.MessageID); ok {
		return msgID, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockBot) SetMyCommands(ctx context.Context, params *telego.SetMyCommandsParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

func (m *MockBot) AnswerCallbackQuery(ctx context.Context, params *telego.AnswerCallbackQueryParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

// Add SendMediaGroup to satisfy telegoapi.BotAPI
func (m *MockBot) SendMediaGroup(ctx context.Context, params *telego.SendMediaGroupParams) ([]telego.Message, error) {
	args := m.Called(ctx, params)
	if msgs, ok := args.Get(0).([]telego.Message); ok {
		return msgs, args.Error(1)
	}
	return nil, args.Error(1)
}

// Add GetChatMember to satisfy telegoapi.BotAPI
func (m *MockBot) GetChatMember(ctx context.Context, params *telego.GetChatMemberParams) (telego.ChatMember, error) {
	args := m.Called(ctx, params)
	if member, ok := args.Get(0).(telego.ChatMember); ok {
		return member, args.Error(1)
	}
	return nil, args.Error(1)
}

// Add SendPhoto to satisfy telegoapi.BotAPI
func (m *MockBot) SendPhoto(ctx context.Context, params *telego.SendPhotoParams) (*telego.Message, error) {
	args := m.Called(ctx, params)
	if msg, ok := args.Get(0).(*telego.Message); ok {
		return msg, args.Error(1)
	}
	return nil, args.Error(1)
}

// Add DeleteMessage to satisfy telegoapi.BotAPI
func (m *MockBot) DeleteMessage(ctx context.Context, params *telego.DeleteMessageParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

// MockUserActionLogger is a mock for UserActionLogger
type MockUserActionLogger struct {
	mock.Mock
}

func (m *MockUserActionLogger) LogUserAction(userID int64, action string, details interface{}) error {
	args := m.Called(userID, action, details)
	return args.Error(0)
}

// MockUserRepository is a mock for UserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) UpdateUser(ctx context.Context, userID int64, username, firstName, lastName string, isAdmin bool, action string) error {
	args := m.Called(ctx, userID, username, firstName, lastName, isAdmin, action)
	return args.Error(0)
}

// MockAdminChecker is a mock implementing the AdminCheckerInterface
type MockAdminChecker struct {
	mock.Mock
}

func (m *MockAdminChecker) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

// MockSuggestionRepository
type MockSuggestionRepository struct {
	mock.Mock
}

func (m *MockSuggestionRepository) CreateSuggestion(ctx context.Context, suggestion *models.Suggestion) error {
	args := m.Called(ctx, suggestion)
	return args.Error(0)
}
func (m *MockSuggestionRepository) GetSuggestionByID(ctx context.Context, id primitive.ObjectID) (*models.Suggestion, error) { // Expect ObjectID
	args := m.Called(ctx, id)
	if sugg, ok := args.Get(0).(*models.Suggestion); ok {
		return sugg, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockSuggestionRepository) UpdateSuggestionStatus(ctx context.Context, id primitive.ObjectID, status string, adminID int64, adminUsername string) error { // Expect ObjectID
	args := m.Called(ctx, id, status, adminID, adminUsername)
	return args.Error(0)
}
func (m *MockSuggestionRepository) GetPendingSuggestions(ctx context.Context, limit int, offset int) ([]models.Suggestion, int64, error) {
	args := m.Called(ctx, limit, offset)
	if suggs, ok := args.Get(0).([]models.Suggestion); ok {
		return suggs, args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *MockSuggestionRepository) DeleteSuggestion(ctx context.Context, id primitive.ObjectID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MockFeedbackRepository
type MockFeedbackRepository struct {
	mock.Mock
}

func (m *MockFeedbackRepository) AddFeedback(ctx context.Context, feedback *models.Feedback) error { // Use models.Feedback
	args := m.Called(ctx, feedback)
	return args.Error(0)
}

// MockSuggestionManager is a mock implementing SuggestionManagerInterface
type MockSuggestionManager struct {
	mock.Mock
	// Add fields for dependencies if needed for methods like HandleSuggestCommand
}

func (m *MockSuggestionManager) GetUserState(userID int64) suggestions.UserState {
	args := m.Called(userID)
	// Handle potential nil return if state isn't mocked
	if args.Get(0) == nil {
		return suggestions.StateIdle // Or a suitable default
	}
	return args.Get(0).(suggestions.UserState)
}
func (m *MockSuggestionManager) SetUserState(userID int64, state suggestions.UserState) {
	m.Called(userID, state)
}
func (m *MockSuggestionManager) HandleSuggestCommand(ctx context.Context, update telego.Update) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// Add HandleReviewCommand to satisfy interface
func (m *MockSuggestionManager) HandleReviewCommand(ctx context.Context, update telego.Update) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// Add HandleFeedbackCommand to satisfy interface
func (m *MockSuggestionManager) HandleFeedbackCommand(ctx context.Context, update telego.Update) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// Renamed from ProcessSuggestionCallback
func (m *MockSuggestionManager) HandleCallbackQuery(ctx context.Context, query telego.CallbackQuery) (bool, error) {
	args := m.Called(ctx, query)
	return args.Bool(0), args.Error(1)
}
func (m *MockSuggestionManager) HandleMessage(ctx context.Context, update telego.Update) (bool, error) {
	args := m.Called(ctx, update)
	return args.Bool(0), args.Error(1)
}

// Add HandleCombinedMediaGroup to satisfy SuggestionManagerInterface
func (m *MockSuggestionManager) HandleCombinedMediaGroup(ctx context.Context, groupID string, messages []telego.Message) error {
	args := m.Called(ctx, groupID, messages)
	return args.Error(0)
}

// --- Test Function ---

func TestHandleStart(t *testing.T) {
	locales.Init()

	// --- Setup Mocks and Handler --- (Use interfaces for adminChecker and suggestionManager)
	mockActionLogger := new(MockUserActionLogger)
	mockUserRepo := new(MockUserRepository)
	mockAdminChecker := new(MockAdminChecker)           // Implements AdminCheckerInterface
	mockSuggestionManager := new(MockSuggestionManager) // Implements SuggestionManagerInterface
	mockFeedbackRepo := new(MockFeedbackRepository)

	// Create MessageHandler using mocks (interfaces are accepted)
	messageHandler := NewMessageHandler(
		12345, // Dummy channel ID
		nil,   // PostLogger (can be nil for start test? check dependencies)
		mockActionLogger,
		mockUserRepo,
		mockSuggestionManager, // Pass interface implementation
		mockAdminChecker,      // Pass interface implementation
		mockFeedbackRepo,
	)

	// --- Test Data ---
	ctx := context.Background()
	testUserID := int64(98765)
	testChatID := int64(54321)
	testUsername := "testuser"
	testFirstName := "Test"
	testLastName := "Userov"

	testMessage := telego.Message{
		MessageID: 100,
		From: &telego.User{
			ID:           testUserID,
			IsBot:        false,
			FirstName:    testFirstName,
			LastName:     testLastName,
			Username:     testUsername,
			LanguageCode: "ru",
		},
		Date: int64(time.Now().Unix()), // FIX: Use int64
		Chat: telego.Chat{
			ID:   testChatID,
			Type: "private",
		},
		Text: "/start",
	}

	// --- Test Cases ---
	t.Run("Success", func(t *testing.T) {
		// Create mocks specific to this subtest for isolation
		subMockBot := new(MockBot) // Implements telegoapi.BotAPI
		subMockActionLogger := new(MockUserActionLogger)
		subMockUserRepo := new(MockUserRepository)
		subMockAdminChecker := new(MockAdminChecker) // Implements AdminCheckerInterface
		// subMockSuggestionManager is not needed for /start, but keep pattern
		// subMockFeedbackRepo is not needed for /start

		// Temporarily replace handler dependencies with subtest mocks
		originalActionLogger := messageHandler.actionLogger
		originalUserRepo := messageHandler.userRepo
		originalAdminChecker := messageHandler.adminChecker // Store original interface
		messageHandler.actionLogger = subMockActionLogger
		messageHandler.userRepo = subMockUserRepo
		messageHandler.adminChecker = subMockAdminChecker // Assign interface implementation
		// Restore original mocks after the test
		defer func() {
			messageHandler.actionLogger = originalActionLogger
			messageHandler.userRepo = originalUserRepo
			messageHandler.adminChecker = originalAdminChecker
		}()

		// --- Mock Expectations ---
		// Expect calls on the INTERFACE mock
		subMockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Once()
		subMockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandStart).Return(nil).Once()
		subMockActionLogger.On("LogUserAction", testUserID, ActionCommandStart, mock.AnythingOfType("map[string]interface {}")).Return(nil).Once()

		// Expect setupCommands call on the bot mock
		subMockBot.On("SetMyCommands", ctx, mock.AnythingOfType("*telego.SetMyCommandsParams")).Return(nil).Once()

		expectedWelcomeMsg := locales.GetMessage(locales.NewLocalizer("ru"), "MsgStart", nil, nil)
		subMockBot.On("SendMessage", ctx, mock.MatchedBy(func(params *telego.SendMessageParams) bool {
			return params.ChatID.ID == testChatID && params.Text == expectedWelcomeMsg
		})).Return(&telego.Message{MessageID: 101}, nil).Once()

		// --- Execute --- (Pass telegoapi.BotAPI implementation)
		err := messageHandler.HandleStart(ctx, subMockBot, testMessage)

		// --- Assert ---
		assert.NoError(t, err)
		subMockBot.AssertExpectations(t)
		subMockActionLogger.AssertExpectations(t)
		subMockUserRepo.AssertExpectations(t)
		subMockAdminChecker.AssertExpectations(t)
	})

	t.Run("SendMessage Error", func(t *testing.T) {
		// Create mocks specific to this subtest
		subMockBot := new(MockBot) // Implements telegoapi.BotAPI
		subMockActionLogger := new(MockUserActionLogger)
		subMockUserRepo := new(MockUserRepository)
		subMockAdminChecker := new(MockAdminChecker) // Implements AdminCheckerInterface

		// Inject mocks
		originalActionLogger := messageHandler.actionLogger
		originalUserRepo := messageHandler.userRepo
		originalAdminChecker := messageHandler.adminChecker
		messageHandler.actionLogger = subMockActionLogger
		messageHandler.userRepo = subMockUserRepo
		messageHandler.adminChecker = subMockAdminChecker // Assign interface
		defer func() {
			messageHandler.actionLogger = originalActionLogger
			messageHandler.userRepo = originalUserRepo
			messageHandler.adminChecker = originalAdminChecker
		}()

		// --- Mock Expectations ---
		subMockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Once()
		subMockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandStart).Return(nil).Once()
		subMockActionLogger.On("LogUserAction", testUserID, ActionCommandStart, mock.AnythingOfType("map[string]interface {}")).Return(nil).Once()

		// Expect setupCommands call
		subMockBot.On("SetMyCommands", ctx, mock.AnythingOfType("*telego.SetMyCommandsParams")).Return(nil).Once()

		expectedError := errors.New("telegram API error")
		subMockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).Return(nil, expectedError).Once()

		// --- Execute --- (Pass telegoapi.BotAPI implementation)
		err := messageHandler.HandleStart(ctx, subMockBot, testMessage)

		// --- Assert ---
		// HandleStart now returns the error from sendSuccess/sendError, which wraps the original error
		// Since sendSuccess logs but returns nil, we expect no error here.
		// If sendError was called (e.g., if setupCommands failed), we'd expect an error.
		// The error from SendMessage within sendSuccess is logged but not returned.
		assert.NoError(t, err)
		subMockBot.AssertExpectations(t)
		subMockActionLogger.AssertExpectations(t)
		subMockUserRepo.AssertExpectations(t)
		subMockAdminChecker.AssertExpectations(t)
	})

	t.Run("SetupCommands Error", func(t *testing.T) {
		// Create mocks specific to this subtest
		subMockBot := new(MockBot) // Implements telegoapi.BotAPI
		subMockActionLogger := new(MockUserActionLogger)
		subMockUserRepo := new(MockUserRepository)
		subMockAdminChecker := new(MockAdminChecker) // Implements AdminCheckerInterface

		// Inject mocks
		originalActionLogger := messageHandler.actionLogger
		originalUserRepo := messageHandler.userRepo
		originalAdminChecker := messageHandler.adminChecker
		messageHandler.actionLogger = subMockActionLogger
		messageHandler.userRepo = subMockUserRepo
		messageHandler.adminChecker = subMockAdminChecker
		defer func() {
			messageHandler.actionLogger = originalActionLogger
			messageHandler.userRepo = originalUserRepo
			messageHandler.adminChecker = originalAdminChecker
		}()

		// --- Mock Expectations ---
		setupError := errors.New("failed to set commands")
		subMockBot.On("SetMyCommands", ctx, mock.AnythingOfType("*telego.SetMyCommandsParams")).Return(setupError).Once()

		// Expect SendMessage call from sendError
		subMockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).Return(nil, nil).Maybe() // Error sending might happen, but test focuses on returning the setupError

		// --- Execute --- (Pass telegoapi.BotAPI implementation)
		err := messageHandler.HandleStart(ctx, subMockBot, testMessage)

		// --- Assert ---
		// HandleStart should return the error from setupCommands, wrapped by sendError
		assert.Error(t, err)
		// Check if the returned error wraps the original setupError
		assert.True(t, errors.Is(err, setupError), "Expected error to wrap the setup error")
		// We don't check logger/user repo mocks as they might not be called if setup fails early
		subMockBot.AssertExpectations(t)
		subMockAdminChecker.AssertNotCalled(t, "IsAdmin", mock.Anything, mock.Anything)
		subMockUserRepo.AssertNotCalled(t, "UpdateUser", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		subMockActionLogger.AssertNotCalled(t, "LogUserAction", mock.Anything, mock.Anything, mock.Anything)

	})

}
