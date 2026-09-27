package database

import (
	"context"
	"os"
	"testing"
	"time"

	"vrcmemes-bot/internal/config"
)

func TestConnectDBRequiresDatabaseAuthentication(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	badURI := os.Getenv("TEST_MONGODB_BAD_URI")
	if uri == "" || badURI == "" {
		t.Skip("set TEST_MONGODB_URI and TEST_MONGODB_BAD_URI for a disposable MongoDB instance")
	}

	client, _, err := ConnectDB(&config.Config{MongoDBURI: uri, MongoDBDatabase: "vrcmemes"})
	if err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Disconnect(ctx); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, _, err := ConnectDB(&config.Config{MongoDBURI: badURI, MongoDBDatabase: "vrcmemes"}); err == nil {
		t.Fatal("invalid credentials accepted")
	}
}
