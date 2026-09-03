//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTimerStoreFixture(t *testing.T) (*RedisTimerStore, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewTimerStore(rdb), mr, rdb
}

func TestRedisTimerStore_EnqueuePopDueRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newTimerStoreFixture(t)

	now := time.Now()
	entries := []TimerEntry{
		{DeviceID: 1, FireAt: now.Add(30 * time.Second)},
		{DeviceID: 2, FireAt: now.Add(2 * time.Minute)},
		{DeviceID: 3, FireAt: now.Add(-time.Minute)}, // already due
	}
	if err := store.Enqueue(ctx, 42, entries); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := store.PopDue(ctx, 42, now, 10)
	if err != nil {
		t.Fatalf("popdue: %v", err)
	}
	if len(got) != 1 || got[0].DeviceID != 3 {
		t.Fatalf("got %v, want exactly the past-due entry", got)
	}

	// After moving the clock forward, the others become due.
	got2, err := store.PopDue(ctx, 42, now.Add(5*time.Minute), 10)
	if err != nil {
		t.Fatalf("popdue2: %v", err)
	}
	if len(got2) != 3 {
		t.Fatalf("got %d entries, want 3", len(got2))
	}
}

func TestRedisTimerStore_EnqueueIsIdempotentOverwrite(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newTimerStoreFixture(t)
	now := time.Now()

	if err := store.Enqueue(ctx, 7, []TimerEntry{{DeviceID: 1, FireAt: now}}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := store.Enqueue(ctx, 7, []TimerEntry{{DeviceID: 2, FireAt: now}}); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	got, err := store.PopDue(ctx, 7, now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("popdue: %v", err)
	}
	if len(got) != 1 || got[0].DeviceID != 2 {
		t.Fatalf("got %v, want only device 2 (second enqueue replaces first)", got)
	}
}

func TestRedisTimerStore_RemoveClearsSchedule(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newTimerStoreFixture(t)
	now := time.Now()
	if err := store.Enqueue(ctx, 9, []TimerEntry{{DeviceID: 1, FireAt: now}}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := store.Remove(ctx, 9); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, err := store.PopDue(ctx, 9, now.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("popdue: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0 after Remove", len(got))
	}
}

func TestRedisTimerStore_RebuildFromSnapshot(t *testing.T) {
	ctx := context.Background()
	store, mr, _ := newTimerStoreFixture(t)

	// Pre-populate with stale data to confirm it is overwritten.
	if err := store.Enqueue(ctx, 1, []TimerEntry{{DeviceID: 99, FireAt: time.Unix(0, 0)}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	now := time.Now()
	snapshot := map[uint][]TimerEntry{
		1: {{DeviceID: 10, FireAt: now}},
		2: {{DeviceID: 20, FireAt: now}, {DeviceID: 21, FireAt: now.Add(time.Minute)}},
	}
	if err := store.Rebuild(ctx, snapshot); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	got1, _ := store.PopDue(ctx, 1, now.Add(time.Hour), 10)
	if len(got1) != 1 || got1[0].DeviceID != 10 {
		t.Fatalf("schedule 1: got %v, want device 10", got1)
	}
	got2, _ := store.PopDue(ctx, 2, now.Add(time.Hour), 10)
	if len(got2) != 2 {
		t.Fatalf("schedule 2: got %d, want 2", len(got2))
	}
	_ = mr
}