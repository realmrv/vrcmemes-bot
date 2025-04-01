package database

import (
	"context"
	"log"
	"time"

	"vrcmemes-bot/config"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var DB *mongo.Database

// ConnectDB устанавливает соединение с MongoDB
func ConnectDB(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.MongoDBURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return err
	}

	// Проверяем соединение
	err = client.Ping(ctx, nil)
	if err != nil {
		return err
	}

	DB = client.Database(cfg.MongoDBName)
	log.Println("Successfully connected to MongoDB")
	return nil
}

// LogUserAction записывает действие пользователя в базу данных
func LogUserAction(userID int64, action string, details interface{}) error {
	collection := DB.Collection("user_actions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, map[string]interface{}{
		"user_id": userID,
		"action":  action,
		"details": details,
		"time":    time.Now(),
	})

	return err
}
