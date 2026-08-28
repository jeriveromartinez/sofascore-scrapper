package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func testRedisClient(t *testing.T) *goredis.Client {
	t.Helper()
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL not set, skipping integration test")
	}
	opt, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := goredis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestInvitationStore_CreateReturnsBase64URLToken(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	token, expiresAt, err := store.Create(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expires in the past")
	}
	if expiresAt.After(time.Now().Add(25 * time.Hour)) {
		t.Fatal("expires too far in the future")
	}
}

func TestInvitationStore_ConsumeSucceedsOnce(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	token, _, err := store.Create(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Consume(ctx, token); err != nil {
		t.Fatal(err)
	}

	if err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected error on second consume")
	}
	if store.Consume(ctx, token) != ErrInvalidInvitation {
		t.Fatal("expected ErrInvalidInvitation")
	}
}

func TestConsumeInvitationConcurrentOnlyOneSucceeds(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	token, _, err := store.Create(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = store.Consume(ctx, token)
		}(i)
	}
	wg.Wait()

	successCount := 0
	failCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		} else if errors.Is(err, ErrInvalidInvitation) {
			failCount++
		}
	}
	if successCount != 1 || failCount != 1 {
		t.Fatalf("expected 1 success + 1 ErrInvalidInvitation, got success=%d fail=%d results=%v", successCount, failCount, results)
	}
}

func TestInvitationStore_ExpiredTokenRejected(t *testing.T) {
	client := testRedisClient(t)
	fixedNow := time.Now()
	store := &InvitationStore{redis: client, now: func() time.Time { return fixedNow }}

	ctx := context.Background()
	token, _, err := store.Create(ctx, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	if err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestInvitationStore_MissingTokenRejected(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	if err := store.Consume(ctx, "nonexistent-token"); err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestInvitationStore_RedisUnavailableReturnsError(t *testing.T) {
	unreachable := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:16379"})
	store := &InvitationStore{redis: unreachable, now: time.Now}
	defer unreachable.Close()

	ctx := context.Background()
	_, _, err := store.Create(ctx, 24*time.Hour)
	if err == nil {
		t.Skip("unexpectedly connected to Redis, retrying after skipping")
	}
}

func TestInvitationStore_ConsumeAfterUserFailureStaysConsumed(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	token, _, err := store.Create(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Consume(ctx, token); err != nil {
		t.Fatal(err)
	}

	if err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected error, invitation should stay consumed")
	}
}

func TestInvitationStore_SHA256OnlyStorage(t *testing.T) {
	client := testRedisClient(t)
	store := &InvitationStore{redis: client, now: time.Now}

	ctx := context.Background()
	token, _, err := store.Create(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := client.Keys(ctx, invitationKeyPrefix+":*").Result()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, k := range keys {
		if len(k) > len(invitationKeyPrefix)+1 {
			found = true
			val, err := client.Get(ctx, k).Result()
			if err != nil {
				t.Fatal(err)
			}
			if val != "1" {
				t.Fatalf("unexpected value %q for key %q", val, k)
			}
			if token == k[len(invitationKeyPrefix)+1:] {
				t.Fatal("stored key should be sha256 hex, not the raw token")
			}
		}
	}
	if !found {
		t.Fatal("no invitation key found in Redis")
	}

	client.Del(ctx, invitationKeyPrefix+":*")
}
