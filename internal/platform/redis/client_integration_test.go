package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

func TestNewPingIntegration(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL not set, skipping integration test")
	}

	cfg := config.Redis{
		URL:          redisURL,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	ctx := context.Background()
	client, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
}
