//go:build integration

package scheduler

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	goredis "github.com/redis/go-redis/v9"
)

func newRunnerTestClient(t *testing.T) *goredis.Client {
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
	client, err := redisplatform.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create redis client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush db: %v", err)
	}
	return client
}

func TestRunLockedThreeRunnersOneExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	client := newRunnerTestClient(t)
	key := "test:runner:one-exec"
	locker := redisplatform.NewLocker(client)

	var counter int32
	job := func(ctx context.Context) error {
		atomic.AddInt32(&counter, 1)
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner := NewRunner(locker, slog.Default())
			err := runner.RunLocked(context.Background(), key, 10*time.Second, job)
			if err != nil {
				t.Errorf("RunLocked error: %v", err)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&counter) != 1 {
		t.Fatalf("expected exactly 1 execution, got %d", counter)
	}
}

func TestRunLockedSkipWithoutLock(t *testing.T) {
	client := newRunnerTestClient(t)
	key := "test:runner:skip"
	locker := redisplatform.NewLocker(client)

	lease, acquired, err := locker.Acquire(context.Background(), key, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("first acquire should succeed")
	}
	defer lease.Release(context.Background())

	var counter int32
	job := func(ctx context.Context) error {
		atomic.AddInt32(&counter, 1)
		return nil
	}

	runner := NewRunner(locker, slog.Default())
	err = runner.RunLocked(context.Background(), key, 10*time.Second, job)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&counter) != 0 {
		t.Fatal("job should not run when lock is held")
	}
}

func TestRunLockedLeaseRenewal(t *testing.T) {
	client := newRunnerTestClient(t)
	key := "test:runner:renewal"
	locker := redisplatform.NewLocker(client)

	started := make(chan struct{})
	completed := make(chan struct{})

	job := func(ctx context.Context) error {
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-completed:
			return nil
		}
	}

	runner := NewRunner(locker, slog.Default())
	go func() {
		_ = runner.RunLocked(context.Background(), key, 750*time.Millisecond, job)
	}()

	<-started

	time.Sleep(500 * time.Millisecond)

	ttl, err := client.PTTL(context.Background(), key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 {
		t.Fatalf("lock should exist (renewed), TTL=%v", ttl)
	}

	close(completed)
}

func TestRunLockedCancelOnLeaseLoss(t *testing.T) {
	client := newRunnerTestClient(t)
	key := "test:runner:cancel-loss"
	locker := redisplatform.NewLocker(client)

	started := make(chan struct{})
	cancelled := make(chan error, 1)

	job := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		cancelled <- ctx.Err()
		return ctx.Err()
	}

	runner := NewRunner(locker, slog.Default())
	go func() {
		_ = runner.RunLocked(context.Background(), key, 500*time.Millisecond, job)
	}()

	<-started

	client.Del(context.Background(), key)

	select {
	case err := <-cancelled:
		if err == nil {
			t.Fatal("expected context error on lease loss")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("job was not cancelled after lease loss")
	}
}

func TestRunLockedReleaseAfterJob(t *testing.T) {
	client := newRunnerTestClient(t)
	key := "test:runner:release"
	locker := redisplatform.NewLocker(client)

	job := func(ctx context.Context) error {
		return nil
	}

	runner := NewRunner(locker, slog.Default())
	err := runner.RunLocked(context.Background(), key, 10*time.Second, job)
	if err != nil {
		t.Fatal(err)
	}

	exists, err := client.Exists(context.Background(), key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists > 0 {
		t.Fatal("lock should be released after job completion")
	}
}
