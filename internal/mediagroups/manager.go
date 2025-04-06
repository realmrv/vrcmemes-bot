package mediagroups

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/mymmrac/telego"
)

const (
	// DefaultProcessDelay specifies the default time to wait before processing a group.
	DefaultProcessDelay = 2 * time.Second
	// DefaultMaxGroupSize limits the number of messages stored per group.
	DefaultMaxGroupSize = 10
)

// ProcessFunc defines the function signature for processing a completed media group.
// It receives the processing context, group ID, and the collected messages.
// It should return an error if processing fails.
type ProcessFunc func(ctx context.Context, groupID string, messages []telego.Message) error

type mediaGroupState struct {
	messages []telego.Message
	timer    *time.Timer
	mu       sync.Mutex
	// firstMsg flag might not be needed if LoadOrStore handles initialization correctly
}

// Manager handles the collection and processing of media groups.
type Manager struct {
	groups sync.Map // map[string]*mediaGroupState
}

// NewManager creates a new media group manager.
func NewManager() *Manager {
	return &Manager{}
}

// HandleMessage adds a message to its group and schedules processing if it's the first message.
// It takes the message, the function to process the group, the delay before processing,
// the maximum size for the group, and an optional parent context for the processor.
func (m *Manager) HandleMessage(
	parentCtx context.Context, // Context from the update handler, for potential cancellation
	message telego.Message,
	handler ProcessFunc,
	delay time.Duration,
	maxSize int,
) error {
	if message.MediaGroupID == "" {
		return nil // Not a media group message
	}

	groupID := message.MediaGroupID

	// Get or create the state for this group ID
	// Initialize state within LoadOrStore using a function for atomicity
	var state *mediaGroupState
	actualVal, _ := m.groups.LoadOrStore(groupID, &mediaGroupState{
		messages: make([]telego.Message, 0, maxSize),
	})
	state = actualVal.(*mediaGroupState)

	// Lock the specific group state for modification
	state.mu.Lock()

	// Add message if not already present and within size limit
	found := false
	for _, msg := range state.messages {
		if msg.MessageID == message.MessageID {
			found = true
			break
		}
	}

	wasEmpty := len(state.messages) == 0
	messageAdded := false

	if !found && len(state.messages) < maxSize {
		state.messages = append(state.messages, message)
		sort.Slice(state.messages, func(i, j int) bool {
			return state.messages[i].MessageID < state.messages[j].MessageID
		})
		messageAdded = true
		log.Printf("[MediaGroupManager Group:%s] Added message %d. Total: %d", groupID, message.MessageID, len(state.messages))
	} else if !found {
		log.Printf("[MediaGroupManager Group:%s] Group limit (%d) reached, message %d dropped.", groupID, maxSize, message.MessageID)
	}

	// Schedule processing only if the group was previously empty and a message was successfully added
	shouldSetTimer := wasEmpty && messageAdded

	state.mu.Unlock() // Unlock before setting the timer

	if shouldSetTimer {
		log.Printf("[MediaGroupManager Group:%s] First message stored. Scheduling processing in %v.", groupID, delay)

		// Re-lock state to safely set the timer
		state.mu.Lock()
		// Check again if timer already exists (rare race condition, but possible)
		if state.timer == nil {
			state.timer = time.AfterFunc(delay, func() {
				// Use background context for processing for now.
				// Passing parentCtx requires more complex management (e.g., linking it to timer cancellation).
				processCtx := context.Background()

				finalMessages := m.getAndRemoveGroup(groupID)
				if finalMessages == nil || len(finalMessages) == 0 {
					log.Printf("[MediaGroupManager Group:%s] Timer fired, but group was empty or already removed.", groupID)
					return
				}

				log.Printf("[MediaGroupManager Group:%s] Timer fired. Processing %d messages.", groupID, len(finalMessages))
				if err := handler(processCtx, groupID, finalMessages); err != nil {
					log.Printf("[MediaGroupManager Group:%s] Error processing group: %v", groupID, err)
					// Consider adding Sentry reporting here
				}
			})
		}
		state.mu.Unlock()
	}

	return nil
}

// getAndRemoveGroup atomically retrieves messages and removes the group state.
func (m *Manager) getAndRemoveGroup(groupID string) []telego.Message {
	val, loaded := m.groups.LoadAndDelete(groupID)
	if !loaded {
		return nil
	}
	state := val.(*mediaGroupState)

	state.mu.Lock()
	defer state.mu.Unlock()

	// Stop the timer if it exists and hasn't fired yet
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil // Clear the timer reference
	}

	// Return a copy of the messages
	msgsCopy := make([]telego.Message, len(state.messages))
	copy(msgsCopy, state.messages)
	return msgsCopy
}

// Shutdown gracefully stops all active media group timers.
func (m *Manager) Shutdown() {
	log.Println("[MediaGroupManager] Shutting down, stopping active timers...")
	stoppedCount := 0
	m.groups.Range(func(key, value interface{}) bool {
		groupID := key.(string)
		state := value.(*mediaGroupState)

		state.mu.Lock()
		if state.timer != nil {
			if state.timer.Stop() { // Stop() returns true if the call stops the timer
				log.Printf("[MediaGroupManager Group:%s] Shutdown: Stopped active timer.", groupID)
				stoppedCount++
			} else {
				// Timer already fired or was stopped
			}
			state.timer = nil // Clear timer reference
		}
		state.mu.Unlock()

		// We can optionally remove the group from the map here during shutdown,
		// but LoadAndDelete in getAndRemoveGroup should handle cleanup eventually.
		// m.groups.Delete(groupID)

		return true // Continue iterating
	})
	log.Printf("[MediaGroupManager] Shutdown complete. Stopped %d active timer(s).", stoppedCount)
}

// TODO: Add a Shutdown method to gracefully stop all active timers.
