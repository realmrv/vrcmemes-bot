package bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"vrcmemes-bot/handlers"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"go.mongodb.org/mongo-driver/mongo"
)

// Bot represents the Telegram bot
type Bot struct {
	bot         *telego.Bot
	handler     *handlers.MessageHandler
	mediaGroups sync.Map
	debug       bool
	stopChan    chan struct{}
}

// New creates a new bot instance
func New(token string, channelID int64, debug bool, db *mongo.Database) (*Bot, error) {
	handler := handlers.NewMessageHandler(channelID, db)

	var bot *telego.Bot
	var err error

	if debug {
		bot, err = telego.NewBot(token, telego.WithDefaultDebugLogger())
	} else {
		bot, err = telego.NewBot(token, telego.WithDefaultLogger(false, false))
	}

	if err != nil {
		return nil, err
	}

	return &Bot{
		bot:      bot,
		handler:  handler,
		debug:    debug,
		stopChan: make(chan struct{}),
	}, nil
}

// storeMessageInGroup stores a message in the media group
func (b *Bot) storeMessageInGroup(message telego.Message) {
	if messages, exists := b.mediaGroups.Load(message.MediaGroupID); exists {
		msgs := messages.([]telego.Message)
		msgs = append(msgs, message)
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].MessageID < msgs[j].MessageID
		})
		b.mediaGroups.Store(message.MediaGroupID, msgs)
	} else {
		b.mediaGroups.Store(message.MediaGroupID, []telego.Message{message})
	}
}

// createInputMedia creates input media array from messages
func (b *Bot) createInputMedia(msgs []telego.Message, caption string) []telego.InputMedia {
	var inputMedia []telego.InputMedia
	for i, msg := range msgs {
		if msg.Photo != nil {
			photo := msg.Photo[len(msg.Photo)-1]
			mediaPhoto := &telego.InputMediaPhoto{
				Type:  "photo",
				Media: telego.InputFile{FileID: photo.FileID},
			}
			if i == 0 && caption != "" {
				mediaPhoto.Caption = caption
			}
			inputMedia = append(inputMedia, mediaPhoto)
		}
	}
	return inputMedia
}

// sendMediaGroupWithRetry sends media group to channel with retry on rate limit
// It now returns the slice of sent messages or an error.
func (b *Bot) sendMediaGroupWithRetry(ctx context.Context, inputMedia []telego.InputMedia, maxRetries int) ([]telego.Message, error) {
	var lastErr error
	var retryCount int

	for retryCount < maxRetries {
		// SendMediaGroup returns a slice of the sent messages
		sentMessages, err := b.bot.SendMediaGroup(ctx, &telego.SendMediaGroupParams{
			ChatID: tu.ID(b.handler.GetChannelID()),
			Media:  inputMedia,
		})

		if err == nil {
			if b.debug {
				log.Printf("Successfully sent media group after %d attempts", retryCount+1)
			}
			return sentMessages, nil // Return sent messages on success
		}

		lastErr = err
		errStr := err.Error()

		// Check if error is "Too Many Requests"
		if strings.Contains(errStr, "Too Many Requests") {
			// Extract retry after value
			var retryAfter int
			_, err := fmt.Sscanf(errStr, "telego: sendMediaGroup: api: 429 \"Too Many Requests: retry after %d\"", &retryAfter)
			if err == nil {
				log.Printf("Rate limit hit (attempt %d/%d), waiting %d seconds", retryCount+1, maxRetries, retryAfter)
				select {
				case <-ctx.Done():
					// Return nil messages and the error
					return nil, fmt.Errorf("context cancelled while waiting for rate limit: %v", lastErr)
				case <-time.After(time.Duration(retryAfter) * time.Second):
					retryCount++
					continue
				}
			}
		}

		// If it's not a rate limit error, return immediately
		// Return nil messages and the error
		return nil, fmt.Errorf("failed to send media group: %v", err)
	}

	// Return nil messages and the final error after max retries
	return nil, fmt.Errorf("max retries (%d) exceeded: %v", maxRetries, lastErr)
}

// processMediaGroup processes a complete media group
func (b *Bot) processMediaGroup(message telego.Message, msgs []telego.Message) {
	log.Printf("Processing media group: ID=%s, Messages count=%d", message.MediaGroupID, len(msgs))

	var caption string
	if caption, _ = b.handler.GetActiveCaption(message.Chat.ID); caption != "" {
		b.handler.StoreMediaGroupCaption(message.MediaGroupID, caption)
	}

	inputMedia := b.createInputMedia(msgs, caption)
	log.Printf("Created input media: count=%d", len(inputMedia))

	if len(inputMedia) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		sentMessages, err := b.sendMediaGroupWithRetry(ctx, inputMedia, 3)
		if err != nil {
			log.Printf("Error sending media group after retries: %v", err)
		} else {
			// Successfully sent the media group
			publishedTime := time.Now()
			channelPostID := 0 // Default value if no messages were sent or ID is unavailable
			if len(sentMessages) > 0 {
				channelPostID = sentMessages[0].MessageID
			}

			// Create log entry
			logEntry := handlers.PostLog{
				SenderID:             message.From.ID,
				SenderUsername:       message.From.Username,
				Caption:              caption,
				MessageType:          "media_group",
				ReceivedAt:           time.Unix(int64(message.Date), 0),
				PublishedAt:          publishedTime,
				ChannelID:            b.handler.GetChannelID(),
				ChannelPostID:        channelPostID,
				OriginalMediaGroupID: message.MediaGroupID,
			}

			// Log to MongoDB via handler
			if err := b.handler.LogPublishedPost(logEntry); err != nil {
				// Error is already logged within LogPublishedPost, but you could add more handling here if needed
				log.Printf("Failed attempt to log post to DB for media group %s", message.MediaGroupID)
			}

			// Optional: Keep a simple log message for console/debug output
			if b.debug {
				log.Printf("Successfully sent and logged media group: %s -> Channel Post ID: %d", message.MediaGroupID, channelPostID)
			}
		}
	} else {
		log.Printf("No valid media found in group")
	}

	b.mediaGroups.Delete(message.MediaGroupID)
	log.Printf("Removed media group from storage: ID=%s", message.MediaGroupID)
}

// handleMediaGroup processes a media group message
func (b *Bot) handleMediaGroup(message telego.Message) {
	log.Printf("Received media group message: ID=%d, MediaGroupID=%s", message.MessageID, message.MediaGroupID)
	b.storeMessageInGroup(message)

	go func() {
		time.Sleep(3 * time.Second)

		if messages, exists := b.mediaGroups.Load(message.MediaGroupID); exists {
			msgs := messages.([]telego.Message)
			if message.MessageID != msgs[0].MessageID {
				log.Printf("Skipping non-first message in group: ID=%d", message.MessageID)
				return
			}
			b.processMediaGroup(message, msgs)
		} else {
			log.Printf("Media group not found in storage: ID=%s", message.MediaGroupID)
		}
	}()
}

// handleCommand processes a single command message
func (b *Bot) handleCommand(ctx *th.Context, message telego.Message) error {
	switch message.Text {
	case "/start":
		return b.handler.HandleStart(ctx, message)
	case "/help":
		return b.handler.HandleHelp(ctx, message)
	case "/status":
		return b.handler.HandleStatus(ctx, message)
	case "/version":
		return b.handler.HandleVersion(ctx, message)
	case "/caption":
		return b.handler.HandleCaption(ctx, message)
	case "/showcaption":
		return b.handler.HandleShowCaption(ctx, message)
	case "/clearcaption":
		return b.handler.HandleClearCaption(ctx, message)
	default:
		return nil
	}
}

// handleMessage processes a single message
func (b *Bot) handleMessage(ctx *th.Context, message telego.Message) error {
	if b.debug {
		log.Printf("Received message: %+v", message)
	}

	if message.MediaGroupID != "" {
		b.handleMediaGroup(message)
		return nil
	}

	if strings.HasPrefix(message.Text, "/") {
		return b.handleCommand(ctx, message)
	}

	if message.Photo != nil {
		return b.handler.HandlePhoto(ctx, message)
	}

	if message.Text != "" {
		return b.handler.HandleText(ctx, message)
	}

	return nil
}

// cleanupOldMediaGroups removes media groups older than 5 minutes
func (b *Bot) cleanupOldMediaGroups() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			now := time.Now()
			b.mediaGroups.Range(func(key, value interface{}) bool {
				msgs := value.([]telego.Message)
				if len(msgs) > 0 {
					// Check if the first message is older than 5 minutes
					if now.Sub(time.Unix(int64(msgs[0].Date), 0)) > 5*time.Minute {
						b.mediaGroups.Delete(key)
						if b.debug {
							log.Printf("Cleaned up old media group: ID=%s", key.(string))
						}
					}
				}
				return true
			})
		}
	}
}

// Start starts the bot
func (b *Bot) Start(ctx context.Context) {
	if b.debug {
		log.Println("Starting bot in debug mode")
	}

	// Start cleanup goroutine
	go b.cleanupOldMediaGroups()

	updates, err := b.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	bh, err := th.NewBotHandler(b.bot, updates)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if b.debug {
			log.Println("Stopping updates")
		}
		close(b.stopChan)
		_ = bh.Stop()
	}()

	bh.HandleMessage(b.handleMessage)

	log.Println("Bot started")
	if err := bh.Start(); err != nil {
		log.Fatal(err)
	}
}
