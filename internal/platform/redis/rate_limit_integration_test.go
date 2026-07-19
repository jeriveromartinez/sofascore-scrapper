package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

func TestRateLimiterAllowIntegration(t *testing.T) {
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

	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	limiter := NewRateLimiter(client)
	policy := RateLimitPolicy{Limit: 10, Window: time.Minute}
	key := "test-ip-1"

	for i := 1; i <= 10; i++ {
		allowed, _, err := limiter.Allow(ctx, key, policy)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !allowed {
			t.Fatalf("request %d: expected allowed, got denied", i)
		}
	}

	allowed, retryAfter, err := limiter.Allow(ctx, key, policy)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("request 11 should be denied")
	}
	if retryAfter <= 0 {
		t.Fatalf("Retry-After should be positive, got %v", retryAfter)
	}
	t.Logf("rate limited with Retry-After: %v", retryAfter)
}

func TestRateLimiterSharedStateIntegration(t *testing.T) {
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

	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	limiter1 := NewRateLimiter(client)
	limiter2 := NewRateLimiter(client)
	policy := RateLimitPolicy{Limit: 3, Window: time.Minute}
	key := "shared-key"

	allowed, _, err := limiter1.Allow(ctx, key, policy)
	if err != nil || !allowed {
		t.Fatalf("limiter1 request 1: allowed=%v err=%v", allowed, err)
	}

	allowed, _, err = limiter2.Allow(ctx, key, policy)
	if err != nil || !allowed {
		t.Fatalf("limiter2 request 1: allowed=%v err=%v", allowed, err)
	}

	allowed, _, err = limiter2.Allow(ctx, key, policy)
	if err != nil || !allowed {
		t.Fatalf("limiter2 request 2: allowed=%v err=%v", allowed, err)
	}

	allowed, _, err = limiter1.Allow(ctx, key, policy)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("request 4 should be denied (shared state)")
	}
}

func TestRateLimiterRedisUnavailableReturnsError(t *testing.T) {
	limiter := NewRateLimiter(nil)
	policy := RateLimitPolicy{Limit: 10, Window: time.Minute}

	allowed, _, err := limiter.Allow(t.Context(), "test-key", policy)
	if err == nil {
		t.Fatal("expected error with nil Redis client")
	}
	if allowed {
		t.Fatal("expected not allowed with nil Redis client")
	}
}
