//go:build integration

package scraper

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
)

// TestService_ScrapeToday_HookReceivesLiveContext is the regression
// test for fix B5 (PR #122). The scraper was passing the errgroup-
// derived ctx (which errgroup cancels on Wait()) into the
// onScrapeComplete hook, so the hook always saw ctx.Err() == context
// canceled. In production the hook calls epoch.Increment which needs
// a live Redis client context; a canceled ctx meant the cache stayed
// on the old epoch until TTL.
func TestService_ScrapeToday_HookReceivesLiveContext(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	// Empty matches so scrapeLeague short-circuits before touching the
	// repo — keeps this test independent of the sqlite fixtures
	// while still exercising the errgroup -> hook path.
	src := &serviceTestFakeSource{matches: nil}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{
		{Source: "fake", SourceLeagueId: "47", Name: "L"},
	}}
	svc, err := NewService(repo, src, cat, 100, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var hookCalls atomic.Int32
	var mu sync.Mutex
	var capturedErr error
	var captured bool
	svc.SetOnScrapeComplete(func(ctx context.Context) error {
		hookCalls.Add(1)
		mu.Lock()
		capturedErr = ctx.Err()
		captured = true
		mu.Unlock()
		return nil
	})

	svc.ScrapeToday(context.Background(), time.Now())

	if got := hookCalls.Load(); got != 1 {
		t.Fatalf("onScrapeComplete call count: want 1, got %d", got)
	}
	mu.Lock()
	gotErr := capturedErr
	gotCaptured := captured
	mu.Unlock()
	if !gotCaptured {
		t.Fatalf("hook was not invoked")
	}
	if gotErr != nil {
		t.Errorf("hook ctx.Err(): want nil (live context), got %v", gotErr)
	}
	if errors.Is(gotErr, context.Canceled) {
		t.Errorf("hook ctx.Err() must not be context.Canceled (errgroup cancellation leaked)")
	}
}

// TestService_ScrapeToday_HookSurvivesAlreadyCanceledParent is the
// edge-case counterpart: when the caller cancels the parent context
// the errgroup is also canceled, and the hook must see the parent
// cancel (not get a synthetic "still alive" context). The fix
// preserves the parent-cancel signal by capturing ctx BEFORE
// errgroup.WithContext.
func TestService_ScrapeToday_HookSurvivesAlreadyCanceledParent(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &serviceTestFakeSource{matches: nil}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{
		{Source: "fake", SourceLeagueId: "47", Name: "L"},
	}}
	svc, err := NewService(repo, src, cat, 100, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var mu sync.Mutex
	var capturedErr error
	var captured bool
	svc.SetOnScrapeComplete(func(ctx context.Context) error {
		mu.Lock()
		capturedErr = ctx.Err()
		captured = true
		mu.Unlock()
		return nil
	})

	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.ScrapeToday(parentCtx, time.Now())

	mu.Lock()
	gotErr := capturedErr
	gotCaptured := captured
	mu.Unlock()
	if !gotCaptured {
		t.Fatalf("hook was not invoked")
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("hook ctx.Err(): want context.Canceled (parent cancelled), got %v", gotErr)
	}
}
