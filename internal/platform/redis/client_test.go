package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
)

// TestNewHonoursCallerDeadline guards against go-redis replacing the
// caller's ctx with context.Background(): a blocking command must give
// up when the caller's deadline passes instead of waiting forever.
func TestNewHonoursCallerDeadline(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := New(context.Background(), config.Redis{
		URL:          "redis://" + mr.Addr(),
		DialTimeout:  time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- client.BLPop(ctx, 0, "client-test:never-filled").Err() }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("BLPOP returned without data and without error")
		}
		var netErr interface{ Timeout() bool }
		if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &netErr) && netErr.Timeout()) {
			t.Fatalf("BLPOP error = %v, want a deadline error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BLPOP ignored the caller's deadline")
	}
}
