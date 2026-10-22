package scraper

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// failingSource is a Source that either fails outright or takes its
// time and then reports whether its context was cancelled meanwhile.
type failingSource struct {
	name      string
	err       error
	delay     time.Duration
	completed atomic.Bool
	cancelled atomic.Bool
}

func (s *failingSource) Name() string { return s.name }

func (s *failingSource) ScheduledEvents(ctx context.Context, league LeagueRef, date time.Time) ([]Match, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			s.cancelled.Store(true)
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	if err := ctx.Err(); err != nil {
		s.cancelled.Store(true)
		return nil, err
	}
	if s.err != nil {
		return nil, s.err
	}
	s.completed.Store(true)
	return nil, nil
}

func (s *failingSource) SearchLeagues(ctx context.Context, query string) ([]LeagueSearchResult, error) {
	return nil, nil
}

// staticCatalog is a CatalogSource serving a fixed set of leagues.
type staticCatalog struct {
	leagues []LeagueRef
}

func (c *staticCatalog) ActiveLeagues(ctx context.Context) ([]LeagueRef, error) {
	return c.leagues, nil
}

func (c *staticCatalog) ActiveLeaguesBySource(ctx context.Context, source string) ([]LeagueRef, error) {
	out := make([]LeagueRef, 0, len(c.leagues))
	for _, l := range c.leagues {
		if l.Source == source {
			out = append(out, l)
		}
	}
	return out, nil
}

// TestScrapeToday_FailingSourceDoesNotCancelSiblings is the
// regression test for the shared errgroup context: the first source
// that failed used to cancel the context of the sources running next
// to it, so one upstream outage silently truncated the other
// upstreams' scrape.
func TestScrapeToday_FailingSourceDoesNotCancelSiblings(t *testing.T) {
	failing := &failingSource{name: "failing", err: errors.New("upstream is down")}
	slow := &failingSource{name: "slow", delay: 150 * time.Millisecond}

	catalog := &staticCatalog{leagues: []LeagueRef{
		{Source: "failing", SourceLeagueId: "1", Sport: "football"},
		{Source: "slow", SourceLeagueId: "2", Sport: "football"},
	}}

	svc, err := NewService(nil, NewSourceDispatcher(failing, slow), catalog, 10, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.ScrapeToday(context.Background(), time.Now())

	if slow.cancelled.Load() {
		t.Error("the sibling source was cancelled by the failing one")
	}
	if !slow.completed.Load() {
		t.Error("the sibling source did not finish its work")
	}
}

// TestNewService_RejectsNilCatalog is the regression test for the
// missing dependency check: a nil catalog used to be accepted and
// only blew up with a nil pointer dereference on the first scrape.
func TestNewService_RejectsNilCatalog(t *testing.T) {
	dispatcher := NewSourceDispatcher(&failingSource{name: "fotmob"})

	if _, err := NewService(nil, dispatcher, nil, 10, 4, slog.Default()); err == nil {
		t.Error("expected an error for a nil catalog")
	}

	var typedNil *staticCatalog
	if _, err := NewService(nil, dispatcher, typedNil, 10, 4, slog.Default()); err == nil {
		t.Error("expected an error for a typed nil catalog")
	}
}

// TestAddDays_KeepsTheCalendarDayAcrossDST is the regression test
// for the 24h lookahead offset: on the days a DST transition falls in,
// a fixed 24h lands on the next calendar day, so the scheduler
// skipped (or repeated) a day of the lookahead window.
func TestAddDays_KeepsTheCalendarDayAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	// 2026-03-08 is the spring-forward day in New York: that day is
	// only 23 hours long.
	before := time.Date(2026, 3, 7, 23, 30, 0, 0, loc)

	got := addDays(before, 1)
	if got.Format("2006-01-02 15:04") != "2026-03-08 23:30" {
		t.Errorf("addDays = %s, want 2026-03-08 23:30", got.Format("2006-01-02 15:04"))
	}

	for i := 1; i <= 7; i++ {
		want := before.AddDate(0, 0, i)
		if got := addDays(before, i); !got.Equal(want) {
			t.Errorf("addDays(%d) = %s, want %s", i, got, want)
		}
	}
}

// recordingSource keeps the dates it was asked to scrape.
type recordingSource struct {
	name  string
	mu    sync.Mutex
	dates []time.Time
}

func (s *recordingSource) Name() string { return s.name }

func (s *recordingSource) ScheduledEvents(ctx context.Context, league LeagueRef, date time.Time) ([]Match, error) {
	s.mu.Lock()
	s.dates = append(s.dates, date)
	s.mu.Unlock()
	return nil, nil
}

func (s *recordingSource) SearchLeagues(ctx context.Context, query string) ([]LeagueSearchResult, error) {
	return nil, nil
}

func (s *recordingSource) scrapedDays() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.dates))
	for _, d := range s.dates {
		out = append(out, d.Format("2006-01-02 15:04"))
	}
	return out
}

// TestScrapeNext7Days_UsesCalendarDaysAcrossDST is the end-to-end
// regression test for the 24h lookahead offset: with a fixed 24h
// offset the window started on the evening before the spring-forward
// transition skipped a calendar day.
func TestScrapeNext7Days_UsesCalendarDaysAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	src := &recordingSource{name: "fotmob"}
	catalog := &staticCatalog{leagues: []LeagueRef{
		{Source: "fotmob", SourceLeagueId: "1", Sport: "football"},
	}}

	svc, err := NewService(nil, NewSourceDispatcher(src), catalog, 10, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.now = func() time.Time {
		return time.Date(2026, 3, 7, 23, 30, 0, 0, loc)
	}
	svc.ScrapeNext7Days(context.Background())

	want := []string{
		"2026-03-08 23:30",
		"2026-03-09 23:30",
		"2026-03-10 23:30",
		"2026-03-11 23:30",
		"2026-03-12 23:30",
		"2026-03-13 23:30",
		"2026-03-14 23:30",
	}
	got := src.scrapedDays()
	if len(got) != len(want) {
		t.Fatalf("scraped %d days, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("day %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestScrapeNext7Days_ReportsTheWaitError is the regression test for
// the discarded g.Wait() error: a failing source in the lookahead
// window was swallowed, so the operator saw a clean run while the
// window was never scraped.
func TestScrapeNext7Days_ReportsTheWaitError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	failing := &failingSource{name: "failing", err: errors.New("upstream is down")}
	catalog := &staticCatalog{leagues: []LeagueRef{
		{Source: "failing", SourceLeagueId: "1", Sport: "football"},
	}}

	svc, err := NewService(nil, NewSourceDispatcher(failing), catalog, 10, 4, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.ScrapeNext7Days(context.Background())

	out := logs.String()
	if !strings.Contains(out, "scrape next 7 days errors") {
		t.Errorf("the lookahead error was not reported, logs:\n%s", out)
	}
	if !strings.Contains(out, "upstream is down") {
		t.Errorf("the underlying error is missing from the logs:\n%s", out)
	}
}
