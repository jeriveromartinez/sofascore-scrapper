package redis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

func newPrefixedClient(t *testing.T, prefix string) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client, err := New(context.Background(), config.Redis{
		URL:       "redis://" + mr.Addr(),
		KeyPrefix: prefix,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// TestKeyPrefix_IsolatesKeys asserts every key the backend writes
// lands under the configured prefix, so two deployments can share one
// Redis instance without sharing rate limit buckets, invitations or
// pub/sub channels.
func TestKeyPrefix_IsolatesKeys(t *testing.T) {
	client, mr := newPrefixedClient(t, "tenant-a:")
	ctx := context.Background()

	if err := client.Set(ctx, "events:current:v1:epoch", "7", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !mr.Exists("tenant-a:events:current:v1:epoch") {
		t.Fatal("the key was not written under the configured prefix")
	}
	if mr.Exists("events:current:v1:epoch") {
		t.Fatal("the key was also written without the prefix")
	}

	got, err := client.Get(ctx, "events:current:v1:epoch").Result()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "7" {
		t.Fatalf("get returned %q, want 7", got)
	}
}

// TestKeyPrefix_AppliesToScriptKeys asserts the Lua scripts the
// backend runs (rate limiter, lease) get their KEYS prefixed too:
// they address the key space directly.
func TestKeyPrefix_AppliesToScriptKeys(t *testing.T) {
	client, mr := newPrefixedClient(t, "tenant-b:")
	ctx := context.Background()

	limiter := NewRateLimiter(client)
	if _, _, err := limiter.Allow(ctx, "user:1", RateLimitPolicy{Limit: 2, Window: time.Minute}); err != nil {
		t.Fatalf("allow: %v", err)
	}

	keys := mr.Keys()
	found := false
	for _, key := range keys {
		if strings.HasPrefix(key, "tenant-b:ratelimit:") {
			found = true
		}
		if strings.HasPrefix(key, "ratelimit:") {
			t.Errorf("key %q was written without the prefix", key)
		}
	}
	if !found {
		t.Fatalf("no rate limit key under the prefix, keys=%v", keys)
	}
}

// TestKeyPrefix_WithoutPrefixLeavesKeysUnchanged asserts the default
// deployment keeps the historical key names.
func TestKeyPrefix_WithoutPrefixLeavesKeysUnchanged(t *testing.T) {
	client, mr := newPrefixedClient(t, "")
	ctx := context.Background()

	if err := client.Set(ctx, "events:current:v1:epoch", "1", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !mr.Exists("events:current:v1:epoch") {
		t.Fatal("the key was not written under its plain name")
	}
}

// TestKeyPrefix_ScanMatchesPrefixedKeys asserts the MATCH pattern of a
// SCAN is prefixed. Without it the scan finds nothing and the orphan
// reprocessing never sees the pending batches.
func TestKeyPrefix_ScanMatchesPrefixedKeys(t *testing.T) {
	client, _ := newPrefixedClient(t, "tenant-c:")
	ctx := context.Background()

	if err := client.Set(ctx, "apk:downloads:pending:1", "a", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := client.Set(ctx, "apk:downloads:pending:2", "b", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := client.Set(ctx, "other:key", "c", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}

	found := map[string]bool{}
	iter := client.Scan(ctx, 0, "apk:downloads:pending:*", 100).Iterator()
	for iter.Next(ctx) {
		found[iter.Val()] = true
	}
	if err := iter.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("scan found %v, want the two pending keys", found)
	}
}

// TestKeyPrefix_ZRangeByScoreReadsPrefixedSet asserts the sorted set
// commands address the prefixed key. A set written with ZADD and read
// with ZRANGEBYSCORE must address the same key, or the read returns
// nothing.
func TestKeyPrefix_ZRangeByScoreReadsPrefixedSet(t *testing.T) {
	client, _ := newPrefixedClient(t, "tenant-d:")
	ctx := context.Background()

	if err := client.ZAdd(ctx, "schedule:1:timers", goredis.Z{Score: 10, Member: "7"}).Err(); err != nil {
		t.Fatalf("zadd: %v", err)
	}

	zs, err := client.ZRangeByScoreWithScores(ctx, "schedule:1:timers", &goredis.ZRangeBy{
		Min: "-inf",
		Max: "100",
	}).Result()
	if err != nil {
		t.Fatalf("zrangebyscore: %v", err)
	}
	if len(zs) != 1 {
		t.Fatalf("zrangebyscore returned %d entries, want 1", len(zs))
	}
}

// TestKeyPrefix_PhysicalKeyIsNotPrefixedTwice asserts that a key
// returned by Redis (SCAN results) can be fed back into the client.
// Prefixing it again would address a key that does not exist.
func TestKeyPrefix_PhysicalKeyIsNotPrefixedTwice(t *testing.T) {
	client, _ := newPrefixedClient(t, "tenant-e:")
	ctx := context.Background()

	if err := client.Set(ctx, "apk:downloads:pending:1", "payload", 0).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := client.Get(ctx, "tenant-e:apk:downloads:pending:1").Result()
	if err != nil {
		t.Fatalf("get physical key: %v", err)
	}
	if got != "payload" {
		t.Fatalf("get returned %q, want payload", got)
	}
}
