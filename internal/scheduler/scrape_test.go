package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingScraper is a scraperService mock that records how many times
// each job is invoked. Used by the cron-driven tests below.
type countingScraper struct {
	todayCalls  atomic.Int64
	futureCalls atomic.Int64
}

func (c *countingScraper) ScrapeToday(_ context.Context, _ time.Time) {
	c.todayCalls.Add(1)
}

func (c *countingScraper) ScrapeNext7Days(_ context.Context) {
	c.futureCalls.Add(1)
}

// TestStartScrape_CronDrivenToday verifies that startScrape drives
// ScrapeToday from a cron entry, not from one of the four manual
// goroutines that the previous implementation spawned. With a 1s
// cadence the test expects at least 2 invocations in 2.5s; the
// previous implementation would produce exactly 1 (from the startup
// one-shot) and no further ticks in the window.
//
// Note: robfig/cron/v3 has sub-second precision quirks on some
// platforms (the docs warn that `time.Ticker` with d < 1s can
// drift), so the test uses 1s for reliability.
func TestStartScrape_CronDrivenToday(t *testing.T) {
	orig := scrapeTodaySpec
	scrapeTodaySpec = "@every 1s"
	t.Cleanup(func() { scrapeTodaySpec = orig })

	cs := &countingScraper{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := slog.New(slog.NewTextHandler(testLogWriter{t}, nil))
	runner := NewRunner(nil, logger)
	var wg sync.WaitGroup
	startScrape(ctx, cs, runner, &wg, logger)

	// Wait long enough for several cron ticks at 1s cadence.
	time.Sleep(2500 * time.Millisecond)

	cancel()
	wg.Wait()

	got := cs.todayCalls.Load()
	if got < 2 {
		t.Errorf("ScrapeToday called %d times in 2.5s with @every 1s spec, want >= 2 (cron is not driving the job)", got)
	}
	if got > 10 {
		t.Errorf("ScrapeToday called %d times in 2.5s, want <= 10 (sanity bound; cron may be misfiring)", got)
	}
}

// TestStartScrape_NilServiceDoesNotPanic verifies the function
// returns cleanly when no scraper service is wired (a valid
// configuration in some deployment topologies).
func TestStartScrape_NilServiceDoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := slog.New(slog.NewTextHandler(testLogWriter{t}, nil))
	runner := NewRunner(nil, logger)
	var wg sync.WaitGroup

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("startScrape panicked with nil service: %v", r)
		}
	}()

	startScrape(ctx, nil, runner, &wg, logger)
	cancel()
	wg.Wait()
}

// TestStartScrape_StopsCleanlyOnContextCancel verifies that cancelling
// the parent context stops the cron without leaking goroutines. The
// shutdown watcher goroutine added by startScrape must exit.
func TestStartScrape_StopsCleanlyOnContextCancel(t *testing.T) {
	cs := &countingScraper{}
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.New(slog.NewTextHandler(testLogWriter{t}, nil))
	runner := NewRunner(nil, logger)
	var wg sync.WaitGroup
	startScrape(ctx, cs, runner, &wg, logger)

	// Let it run briefly.
	time.Sleep(50 * time.Millisecond)

	cancel()

	// wg.Wait must return promptly. Use a short timeout to detect leaks.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// clean shutdown
	case <-time.After(2 * time.Second):
		t.Fatal("startScrape did not stop within 2s of context cancel (goroutine leak?)")
	}
}

// testLogWriter is an io.Writer that forwards to t.Log so slog output
// shows up in the test results instead of being silently discarded.
type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
