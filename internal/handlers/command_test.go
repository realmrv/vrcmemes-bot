package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"vrcmemes-bot/internal/database/models" // Add import for models
	"vrcmemes-bot/internal/locales"         // Add mediagroups import
	"vrcmemes-bot/internal/suggestions"
	"vrcmemes-bot/pkg/utils" // Import utils for escaping

	// Import for BotAPI
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil" // Import for telegoutil
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
	// Allow returning nil message for successful calls if needed by tests
	if args.Get(0) == nil {
		return nil, args.Error(1)
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

// --- Test Suite Setup ---

const (
	testChannelID = int64(12345)
	testVersion   = "v1.2.3-test"
)

type testHandlerSuite struct {
	t                     *testing.T
	mockBot               *MockBot
	mockActionLogger      *MockUserActionLogger
	mockUserRepo          *MockUserRepository
	mockAdminChecker      *MockAdminChecker
	mockSuggestionManager *MockSuggestionManager
	mockFeedbackRepo      *MockFeedbackRepository
	handler               *MessageHandler
}

// setupTestHandlerSuite creates a new suite with fresh mocks and handler instance.
func setupTestHandlerSuite(t *testing.T) *testHandlerSuite {
	t.Helper()

	mockBot := new(MockBot)
	mockActionLogger := new(MockUserActionLogger)
	mockUserRepo := new(MockUserRepository)
	mockAdminChecker := new(MockAdminChecker)
	mockSuggestionManager := new(MockSuggestionManager)
	mockFeedbackRepo := new(MockFeedbackRepository)

	var activeCaptionsMap sync.Map

	handler := &MessageHandler{
		channelID:         testChannelID,
		postLogger:        nil,
		actionLogger:      mockActionLogger,
		userRepo:          mockUserRepo,
		suggestionManager: mockSuggestionManager,
		adminChecker:      mockAdminChecker,
		feedbackRepo:      mockFeedbackRepo,
		version:           testVersion,
		activeCaptions:    activeCaptionsMap,
	}

	// Initialize the commands slice using the local Command type and localization KEYS
	handler.commands = []Command{ // Use the local Command type
		{Command: "start", Description: "CmdStartDesc", Handler: nil},
		{Command: "help", Description: "CmdHelpDesc", Handler: nil},
		{Command: "status", Description: "CmdStatusDesc", Handler: nil},
		{Command: "version", Description: "CmdVersionDesc", Handler: nil},
		{Command: "caption", Description: "CmdCaptionDesc", Handler: nil},
		{Command: "showcaption", Description: "CmdShowCaptionDesc", Handler: nil},
		{Command: "clearcaption", Description: "CmdClearCaptionDesc", Handler: nil},
		{Command: "suggest", Description: "CmdSuggestDesc", Handler: nil},
		{Command: "review", Description: "CmdReviewDesc", Handler: nil},
		{Command: "feedback", Description: "CmdFeedbackDesc", Handler: nil},
	}

	return &testHandlerSuite{
		t:                     t,
		mockBot:               mockBot,
		mockActionLogger:      mockActionLogger,
		mockUserRepo:          mockUserRepo,
		mockAdminChecker:      mockAdminChecker,
		mockSuggestionManager: mockSuggestionManager,
		mockFeedbackRepo:      mockFeedbackRepo,
		handler:               handler,
	}
}

// --- Test Functions (Refactored) ---

func TestHandleStart(t *testing.T) {
	locales.Init("en")            // Ensure locales are initialized for GetMessage
	s := setupTestHandlerSuite(t) // Setup the suite

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
			Username:     testUsername,
			FirstName:    testFirstName,
			LastName:     testLastName,
			LanguageCode: "en",
		},
		Chat: telego.Chat{ID: testChatID},
		Date: int64(time.Now().Unix()),
		Text: "/start",
	}

	t.Run("HandleStart", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Arrange
			// Use NewLocalizer to get the expected text based on the test user's language
			expectedMsgText := locales.GetMessage(locales.NewLocalizer("en"), "MsgStart", nil, nil)
			expectedEscapedText := utils.EscapeMarkdownV2(expectedMsgText)

			// Expect admin check (used for RecordUserActivity)
			s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Once()

			// Expect logging & user update with correct Action constant
			s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandStart, mock.Anything).Return(nil).Once()
			s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandStart).Return(nil).Once() // Use ActionCommandStart

			// Expect SetMyCommands call (part of HandleStart's setupCommands)
			s.mockBot.On("SetMyCommands", ctx, mock.AnythingOfType("*telego.SetMyCommandsParams")).Return(nil).Once()

			// Expect SendMessage call, capture the params
			var capturedParams *telego.SendMessageParams
			s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
				Run(func(args mock.Arguments) {
					if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
						capturedParams = params
					}
				}).
				Return(&telego.Message{}, nil).Once() // Expect one call

			// Act
			err := s.handler.HandleStart(ctx, s.mockBot, testMessage)

			// Assert
			assert.NoError(t, err)
			s.mockAdminChecker.AssertExpectations(t)
			s.mockActionLogger.AssertExpectations(t)
			s.mockUserRepo.AssertExpectations(t)
			s.mockBot.AssertExpectations(t)

			assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
			if capturedParams != nil {
				assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
				assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
				assert.Equal(t, expectedEscapedText, capturedParams.Text)
			}
		})
	})
}

func TestHandleHelp(t *testing.T) {
	locales.Init("en")
	s := setupTestHandlerSuite(t) // Setup the suite

	// --- Test Data ---
	ctx := context.Background()
	testUserID := int64(11111)
	testChatID := int64(22222)
	testUsername := "helpuser"
	testFirstName := "Helpful"
	testLastName := "Helper"
	testMessage := telego.Message{
		MessageID: 200,
		From: &telego.User{
			ID:           testUserID,
			Username:     testUsername,
			FirstName:    testFirstName,
			LastName:     testLastName,
			LanguageCode: "en",
		},
		Chat: telego.Chat{ID: testChatID},
		Date: int64(time.Now().Unix()),
		Text: "/help",
	}

	t.Run("HandleHelp", func(t *testing.T) {
		t.Run("AdminUser", func(t *testing.T) {
			// Arrange
			localizer := locales.NewLocalizer("en")
			var helpTextBuilder strings.Builder
			helpTextBuilder.WriteString(locales.GetMessage(localizer, "MsgHelpHeader", nil, nil) + "\n")
			// Use handler's commands for dynamic list building
			for _, cmd := range s.handler.commands {
				if cmd.Command != "suggest" && cmd.Command != "feedback" { // Admin filter
					localizedDesc := locales.GetMessage(localizer, cmd.Description, nil, nil)
					helpTextBuilder.WriteString(fmt.Sprintf("/%s - %s\n", cmd.Command, localizedDesc))
				}
			}
			// Footer: Append directly without extra newline
			helpTextBuilder.WriteString(locales.GetMessage(localizer, "MsgHelpFooterAdmin", nil, nil))
			expectedEscapedText := utils.EscapeMarkdownV2(helpTextBuilder.String())

			s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(true, nil).Once()
			s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandHelp, mock.Anything).Return(nil).Once()
			s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, true, ActionCommandHelp).Return(nil).Once() // Use ActionCommandHelp

			// Expect SendMessage call, capture the params
			var capturedParams *telego.SendMessageParams
			s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
				Run(func(args mock.Arguments) {
					if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
						capturedParams = params
					}
				}).
				Return(&telego.Message{}, nil).Once()

			// Act
			err := s.handler.HandleHelp(ctx, s.mockBot, testMessage)

			// Assert
			assert.NoError(t, err)
			s.mockAdminChecker.AssertExpectations(t)
			s.mockActionLogger.AssertExpectations(t)
			s.mockUserRepo.AssertExpectations(t)
			s.mockBot.AssertExpectations(t)
			assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
			if capturedParams != nil {
				assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
				assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
				assert.Equal(t, expectedEscapedText, capturedParams.Text)
			}
		})

		t.Run("RegularUser", func(t *testing.T) {
			// Arrange
			localizer := locales.NewLocalizer("en")
			var helpTextBuilder strings.Builder
			helpTextBuilder.WriteString(locales.GetMessage(localizer, "MsgHelpHeader", nil, nil) + "\n")
			// Use handler's commands for dynamic list building
			for _, cmd := range s.handler.commands {
				if cmd.Command == "start" || cmd.Command == "suggest" || cmd.Command == "feedback" { // Regular user filter
					localizedDesc := locales.GetMessage(localizer, cmd.Description, nil, nil)
					helpTextBuilder.WriteString(fmt.Sprintf("/%s - %s\n", cmd.Command, localizedDesc))
				}
			}
			// Footer: Append directly without extra newline
			helpTextBuilder.WriteString(locales.GetMessage(localizer, "MsgHelpFooterUser", nil, nil))
			expectedEscapedText := utils.EscapeMarkdownV2(helpTextBuilder.String())

			s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Once()
			s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandHelp, mock.Anything).Return(nil).Once()
			s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandHelp).Return(nil).Once() // Use ActionCommandHelp

			// Expect SendMessage call, capture the params
			var capturedParams *telego.SendMessageParams
			s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
				Run(func(args mock.Arguments) {
					if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
						capturedParams = params
					}
				}).
				Return(&telego.Message{}, nil).Once()

			// Act
			err := s.handler.HandleHelp(ctx, s.mockBot, testMessage)

			// Assert
			assert.NoError(t, err)
			s.mockAdminChecker.AssertExpectations(t)
			s.mockActionLogger.AssertExpectations(t)
			s.mockUserRepo.AssertExpectations(t)
			s.mockBot.AssertExpectations(t)
			assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
			if capturedParams != nil {
				assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
				assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
				assert.Equal(t, expectedEscapedText, capturedParams.Text)
			}
		})
	})
}

func TestHandleStatus(t *testing.T) {
	locales.Init("en")
	s := setupTestHandlerSuite(t) // Setup the suite

	// --- Test Data ---
	ctx := context.Background()
	testUserID := int64(33333)
	testChatID := int64(44444)
	testUsername := "statususer"
	testFirstName := "Status"
	testLastName := "Checker"
	testMessage := telego.Message{
		MessageID: 300,
		From: &telego.User{
			ID:           testUserID,
			Username:     testUsername,
			FirstName:    testFirstName,
			LastName:     testLastName,
			LanguageCode: "en",
		},
		Chat: telego.Chat{ID: testChatID},
		Date: int64(time.Now().Unix()),
		Text: "/status",
	}

	t.Run("HandleStatus", func(t *testing.T) {
		t.Run("WithCaption", func(t *testing.T) {
			// Arrange
			activeCaption := "Test Caption"
			// Manually store the caption in the handler's map for this test
			s.handler.activeCaptions.Store(testChatID, activeCaption)
			// Ensure cleanup after test
			defer s.handler.activeCaptions.Delete(testChatID)

			localizer := locales.NewLocalizer("en")
			// Call GetMessage WITH template data
			statusText := locales.GetMessage(localizer, "MsgStatus", map[string]interface{}{
				"ChannelID": s.handler.channelID, // Use channelID from suite
				"Caption":   activeCaption,
			}, nil)
			expectedEscapedText := utils.EscapeMarkdownV2(statusText)

			s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandStatus, mock.Anything).Return(nil).Once()
			s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandStatus).Return(nil).Once() // Use ActionCommandStatus
			s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Maybe()
			// No mock needed for GetActiveCaption as it's internal to MessageHandler

			// Expect SendMessage call, capture the params
			var capturedParams *telego.SendMessageParams
			s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
				Run(func(args mock.Arguments) {
					if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
						capturedParams = params
					}
				}).
				Return(&telego.Message{}, nil).Once()

			// Act
			err := s.handler.HandleStatus(ctx, s.mockBot, testMessage)

			// Assert
			assert.NoError(t, err)
			s.mockActionLogger.AssertExpectations(t)
			s.mockUserRepo.AssertExpectations(t)
			// mockSuggestionManager.AssertExpectations(t) // No need to assert this mock
			s.mockBot.AssertExpectations(t)
			assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
			if capturedParams != nil {
				assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
				assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
				assert.Equal(t, expectedEscapedText, capturedParams.Text)
			}
		})

		t.Run("WithoutCaption", func(t *testing.T) {
			// Arrange
			s.handler.activeCaptions.Delete(testChatID)
			localizer := locales.NewLocalizer("en")
			// Get the final status message WITH AN EMPTY STRING substituted for Caption
			statusTextWithEmptyCaption := locales.GetMessage(localizer, "MsgStatus", map[string]interface{}{
				"ChannelID": s.handler.channelID, // Use channelID from suite
				"Caption":   "",                  // Use empty string, matching handler logic
			}, nil)
			// Escape the final resulting string
			expectedEscapedText := utils.EscapeMarkdownV2(statusTextWithEmptyCaption)

			s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandStatus, mock.Anything).Return(nil).Once()
			s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandStatus).Return(nil).Once() // Use ActionCommandStatus
			s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Maybe()
			// No mock needed for GetActiveCaption

			// Expect SendMessage call, capture the params
			var capturedParams *telego.SendMessageParams
			s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
				Run(func(args mock.Arguments) {
					if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
						capturedParams = params
					}
				}).
				Return(&telego.Message{}, nil).Once()

			// Act
			err := s.handler.HandleStatus(ctx, s.mockBot, testMessage)

			// Assert
			assert.NoError(t, err)
			s.mockActionLogger.AssertExpectations(t)
			s.mockUserRepo.AssertExpectations(t)
			// mockSuggestionManager.AssertExpectations(t) // No need to assert this mock
			s.mockBot.AssertExpectations(t)
			assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
			if capturedParams != nil {
				assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
				assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
				assert.Equal(t, expectedEscapedText, capturedParams.Text)
			}
		})
	})
}

func TestHandleVersion(t *testing.T) {
	locales.Init("en")
	s := setupTestHandlerSuite(t) // Setup the suite

	// --- Test Data ---
	ctx := context.Background()
	testUserID := int64(55555)
	testChatID := int64(66666)
	testUsername := "versionuser"
	testFirstName := "Version"
	testLastName := "Info"
	testMessage := telego.Message{
		MessageID: 400,
		From: &telego.User{
			ID:           testUserID,
			Username:     testUsername,
			FirstName:    testFirstName,
			LastName:     testLastName,
			LanguageCode: "en",
		},
		Chat: telego.Chat{ID: testChatID},
		Date: int64(time.Now().Unix()),
		Text: "/version",
	}

	t.Run("HandleVersion", func(t *testing.T) {
		// Arrange
		localizer := locales.NewLocalizer("en")
		// Call GetMessage WITH template data
		versionText := locales.GetMessage(localizer, "MsgVersion", map[string]interface{}{
			"Version": s.handler.version, // Use version from suite
		}, nil)
		expectedEscapedText := utils.EscapeMarkdownV2(versionText)

		s.mockActionLogger.On("LogUserAction", testUserID, ActionCommandVersion, mock.Anything).Return(nil).Once()
		s.mockUserRepo.On("UpdateUser", ctx, testUserID, testUsername, testFirstName, testLastName, false, ActionCommandVersion).Return(nil).Once() // Use ActionCommandVersion
		s.mockAdminChecker.On("IsAdmin", ctx, testUserID).Return(false, nil).Maybe()

		// Expect SendMessage call, capture the params
		var capturedParams *telego.SendMessageParams
		s.mockBot.On("SendMessage", ctx, mock.AnythingOfType("*telego.SendMessageParams")).
			Run(func(args mock.Arguments) {
				if params, ok := args.Get(1).(*telego.SendMessageParams); ok {
					capturedParams = params
				}
			}).
			Return(&telego.Message{}, nil).Once()

		// Act
		err := s.handler.HandleVersion(ctx, s.mockBot, testMessage)

		// Assert
		assert.NoError(t, err)
		s.mockActionLogger.AssertExpectations(t)
		s.mockUserRepo.AssertExpectations(t)
		// mockAdminChecker.AssertExpectations(t) // Not strictly needed as it's .Maybe()
		s.mockBot.AssertExpectations(t)
		assert.NotNil(t, capturedParams, "SendMessage parameters were not captured")
		if capturedParams != nil {
			assert.Equal(t, telegoutil.ID(testChatID), capturedParams.ChatID)
			assert.Equal(t, telego.ModeMarkdownV2, capturedParams.ParseMode)
			assert.Equal(t, expectedEscapedText, capturedParams.Text)
		}
	})
}

// TODO: Add tests for HandleCaption, HandleShowCaption, HandleClearCaption if needed
