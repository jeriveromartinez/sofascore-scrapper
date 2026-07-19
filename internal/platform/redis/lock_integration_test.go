package redis

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

func newLeaseTestClient(t *testing.T) *goredis.Client {
	t.Helper()
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
		t.Fatalf("create redis client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush db: %v", err)
	}
	return client
}

func TestLeaseAcquireOneWinner(t *testing.T) {
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:one-winner"

	got, acquired, err := locker.Acquire(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("first acquire should succeed")
	}
	if got == nil {
		t.Fatal("lease should not be nil")
	}
	if got.Key() != key {
		t.Fatalf("expected key %q, got %q", key, got.Key())
	}
	if got.Owner() == "" {
		t.Fatal("owner should not be empty")
	}

	_, acquired, err = locker.Acquire(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		t.Fatal("second acquire should fail while held")
	}

	if err := got.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseWrongOwnerCannotRenew(t *testing.T) {
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:wrong-renew"

	_, acquired, err := locker.Acquire(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("acquire should succeed")
	}

	wrongLease := &lease{key: key, owner: "wrong-owner", client: client}
	err = wrongLease.Renew(ctx, 5*time.Second)
	if err == nil {
		t.Fatal("expected error renewing with wrong owner")
	}
	if err != ErrLeaseLost {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
}

func TestLeaseWrongOwnerCannotRelease(t *testing.T) {
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:wrong-release"

	got, acquired, err := locker.Acquire(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("acquire should succeed")
	}

	wrongLease := &lease{key: key, owner: "wrong-owner", client: client}
	err = wrongLease.Release(ctx)
	if err == nil {
		t.Fatal("expected error releasing with wrong owner")
	}
	if err != ErrLeaseLost {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}

	if err := got.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseRenewExtendsTTL(t *testing.T) {
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:renew-ttl"

	got, acquired, err := locker.Acquire(ctx, key, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("acquire should succeed")
	}

	if err := got.Renew(ctx, 30*time.Second); err != nil {
		t.Fatalf("renew failed: %v", err)
	}

	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl < 20*time.Second {
		t.Fatalf("TTL should be extended to ~30s, got %v", ttl)
	}

	if err := got.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseExpiryPermitsTakeover(t *testing.T) {
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:expiry-takeover"

	got, acquired, err := locker.Acquire(ctx, key, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("first acquire should succeed")
	}

	time.Sleep(200 * time.Millisecond)

	_, acquired, err = locker.Acquire(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("second acquire should succeed after expiry")
	}

	err = got.Renew(ctx, 10*time.Second)
	if err == nil {
		t.Fatal("renew on expired lease should fail")
	}
	if err != ErrLeaseLost {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
}

func TestLeaseRedisUnavailableReturnsError(t *testing.T) {
	locker := NewLocker(nil)
	_, acquired, err := locker.Acquire(t.Context(), "test:unavailable", time.Minute)
	if err == nil {
		t.Fatal("expected error when Redis is nil")
	}
	if acquired {
		t.Fatal("should not acquire when Redis is nil")
	}
}

func TestLeaseHundredConcurrentOneWinner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}
	client := newLeaseTestClient(t)
	locker := NewLocker(client)
	ctx := context.Background()
	key := "test:concurrent"

	var winners int32
	var wg sync.WaitGroup
	concurrency := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, acquired, err := locker.Acquire(ctx, key, 10*time.Second)
			if err != nil {
				t.Errorf("acquire error: %v", err)
				return
			}
			if acquired {
				atomic.AddInt32(&winners, 1)
			}
		}()
	}
	wg.Wait()

	count := atomic.LoadInt32(&winners)
	if count != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", count)
	}
}
