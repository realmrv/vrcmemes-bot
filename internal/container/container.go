package container

import (
	"context"
	"fmt"
	"log"
	"time"
	"vrcmemes-bot/internal/auth"
	"vrcmemes-bot/internal/config"
	"vrcmemes-bot/internal/database"
	"vrcmemes-bot/internal/handlers"
	"vrcmemes-bot/internal/mediagroups"
	"vrcmemes-bot/internal/suggestions"

	"github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/mongo"
)

// Container holds all the application's dependencies.
type Container struct {
	Cfg            *config.Config
	MongoClient    *mongo.Client
	DB             *mongo.Database
	Bot            *telego.Bot
	SuggestionRepo database.SuggestionRepository
	ActionLogger   database.UserActionLogger
	PostLogger     database.PostLogger
	UserRepo       database.UserRepository
	FeedbackRepo   database.FeedbackRepository
	AdminChecker   *auth.AdminChecker
	MediaGroupMgr  *mediagroups.Manager
	SuggestionMgr  *suggestions.Manager
	MessageHandler *handlers.MessageHandler
}

// New creates and initializes a new Container.
// It encapsulates the logic of creating all dependencies.
func New(cfg *config.Config) (*Container, error) {
	c := &Container{
		Cfg: cfg,
	}
	initialized := false
	defer func() {
		if !initialized {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			c.Close(ctx)
		}
	}()

	var err error

	// Connect to Database
	c.MongoClient, c.DB, err = connectDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create Repositories
	c.createRepositories()

	// Create Media Group Manager
	c.MediaGroupMgr = mediagroups.NewManager()

	// Create Telego Bot
	botOpts := []telego.BotOption{telego.WithDefaultLogger(false, false)}
	if cfg.Debug {
		botOpts = []telego.BotOption{telego.WithDefaultDebugLogger()}
	}
	c.Bot, err = telego.NewBot(cfg.BotToken, botOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telego bot: %w", err)
	}

	// Setup Core Bot Components
	err = c.setupBotComponents()
	if err != nil {
		return nil, fmt.Errorf("failed to setup bot components: %w", err)
	}

	initialized = true
	return c, nil
}

// connectDatabase establishes a connection to MongoDB.
func connectDatabase(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	client, _, err := database.ConnectDB(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	log.Println("Connected to MongoDB.")
	db := client.Database(cfg.MongoDBDatabase)
	return client, db, nil
}

// createRepositories initializes all necessary database repositories.
func (c *Container) createRepositories() {
	c.SuggestionRepo = database.NewMongoSuggestionRepository(c.DB)
	c.ActionLogger = database.NewMongoLogger(c.DB)
	c.PostLogger = database.NewMongoLogger(c.DB)
	c.UserRepo = database.NewMongoLogger(c.DB)
	c.FeedbackRepo = database.NewFeedbackRepository(c.DB)
}

// setupBotComponents creates the core application components.
func (c *Container) setupBotComponents() error {
	var err error
	c.AdminChecker, err = auth.NewAdminChecker(c.Bot, c.Cfg.ChannelID)
	if err != nil {
		return fmt.Errorf("failed to create admin checker: %w", err)
	}

	c.SuggestionMgr = suggestions.NewManager(
		c.Bot,
		c.SuggestionRepo,
		c.Cfg.ChannelID,
		c.AdminChecker,
		c.FeedbackRepo,
		c.MediaGroupMgr,
	)

	c.MessageHandler = handlers.NewMessageHandler(
		c.Cfg.ChannelID,
		c.PostLogger,
		c.ActionLogger,
		c.UserRepo,
		c.SuggestionMgr,
		c.AdminChecker,
		c.FeedbackRepo,
		c.Cfg.Version,
	)

	return nil
}

// Close handles the graceful shutdown of container resources, like the database connection.
func (c *Container) Close(ctx context.Context) {
	if c.MediaGroupMgr != nil {
		c.MediaGroupMgr.Shutdown()
	}

	if c.MongoClient != nil {
		log.Println("Attempting to disconnect from MongoDB...")
		if err := c.MongoClient.Disconnect(ctx); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		} else {
			log.Println("Disconnected from MongoDB.")
		}
	}
}
