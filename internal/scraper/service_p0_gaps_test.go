//go:build integration

package scraper

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
)

// newCaptureLog returns a logger whose text output is captured in buf.
// Tests grep buf for substrings to assert specific log records.
func newCaptureLog() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

// lineContainsAll returns true when at least one line in buf contains
// every needle as a substring.
func lineContainsAll(buf *bytes.Buffer, needles ...string) bool {
	for _, line := range strings.Split(buf.String(), "\n") {
		ok := true
		for _, n := range needles {
			if !strings.Contains(line, n) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// p0Match returns a Match with non-football Sport so the football
// filter inside dispatchDayMatch's auto-create block does not mask
// the per-source gate under test.
func p0Match(id, leagueID, source string) Match {
	return Match{
		Source:         source,
		SourceMatchId:  id,
		Slug:           "x-" + id,
		StartTimestamp: time.Now(),
		Status:         MatchStatus{Type: "scheduled"},
		HomeTeam:       Team{SourceId: 1, Name: "H"},
		AwayTeam:       Team{SourceId: 2, Name: "A"},
		League:         LeagueRef{Source: source, SourceLeagueId: leagueID, Name: "L", Sport: "tennis"},
	}
}

// TestService_PerSourceEmptyCatalogWarns characterizes gap P0 #2:
// ScrapeToday silently skips a source whose ActiveLeaguesBySource
// returns an empty bucket (service.go:161-163 contains only a bare
// `continue`). With one source empty and another non-empty, no log
// names the empty source. The fix should emit a per-source warning.
func TestService_PerSourceEmptyCatalogWarns(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	s365 := &routingFakeSource{name: "scores365"}
	fm := &routingFakeSource{name: "fotmob"}
	dispatcher := NewSourceDispatcher(s365, fm)

	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{
		{Source: "scores365", SourceLeagueId: "47", Name: "PL"},
	}}

	logger, buf := newCaptureLog()
	svc, err := NewService(repo, dispatcher, cat, 100, 1, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	if lineContainsAll(buf, "level=WARN", "source=fotmob") {
		return
	}
	t.Fatalf("expected a per-source warning mentioning source=fotmob (empty catalog bucket)\nlog:\n%s", buf.String())
}

// TestService_FotMobUnknownLeagueProducesLog characterizes gap P0 #1:
// dispatchDayMatch auto-creates unknown leagues only when
// src.Name() == "scores365" (service.go:319). A non-scores365 source
// like fotmob that returns matches for league IDs not in the catalog
// drops them silently with no log. The fix should surface the unknown
// FotMob league (via log) OR persist its matches.
func TestService_FotMobUnknownLeagueProducesLog(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &dayMatchingFakeSource{
		name: "fotmob",
		matches: []Match{
			p0Match("fm1", "47", "fotmob"),
			p0Match("fm2", "999", "fotmob"),
		},
	}
	cat := &ensureFakeCatalog{
		leagues: []LeagueRef{
			{Source: "fotmob", SourceLeagueId: "47", Name: "PL", Sport: "tennis"},
		},
	}
	logger, buf := newCaptureLog()
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	if lineContainsAll(buf, "999") {
		return
	}
	var persisted int64
	if err := db.Model(&events.Event{}).Where("league_id = ?", 999).Count(&persisted).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if persisted > 0 {
		return
	}
	t.Fatalf("expected either a log line mentioning league 999 OR the 999 match to be persisted; both fail\nlog:\n%s", buf.String())
}

// TestService_UpsertLeagueMatchesLogsZeroMatches characterizes gap P0 #3:
// upsertLeagueMatches short-circuits at service.go:424 with a bare
// `return nil` when the bucket is empty. Operators get no signal that
// "this league returned zero matches today". The fix should emit a log
// record naming the empty bucket.
func TestService_UpsertLeagueMatchesLogsZeroMatches(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &dayMatchingFakeSource{name: "scores365", matches: nil}
	cat := &ensureFakeCatalog{leagues: []LeagueRef{
		{Source: "scores365", SourceLeagueId: "47", Name: "PL"},
	}}
	logger, buf := newCaptureLog()
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	league := LeagueRef{Source: "scores365", SourceLeagueId: "47", Name: "PL", Sport: "tennis"}
	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	if err := svc.upsertLeagueMatches(context.Background(), league, fixedNow, nil); err != nil {
		t.Fatalf("upsertLeagueMatches: unexpected error for empty input: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatalf("expected a log entry for zero-matches bucket; nothing was emitted\nlog:\n%s", buf.String())
	}
	if lineContainsAll(buf, "47") {
		return
	}
	t.Fatalf("expected a log entry mentioning league 47 for zero-matches bucket; only unrelated logs were emitted\nlog:\n%s", buf.String())
}

// TestService_NonScores365AutoCreateLogged characterizes gap P0 #4:
// the auto-create gate at service.go:319 (`src.Name() == "scores365"`)
// silently drops matches that any non-scores365 source returns for
// league IDs not in the catalog. With a DayMatcher source named
// "mystery_dm", the gate fires and byLeague["999"] is never visited.
// The fix should surface the unknown league (via log) OR auto-create it.
func TestService_NonScores365AutoCreateLogged(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &dayMatchingFakeSource{
		name: "mystery_dm",
		matches: []Match{
			p0Match("md1", "47", "mystery_dm"),
			p0Match("md2", "999", "mystery_dm"),
		},
	}
	cat := &ensureFakeCatalog{
		leagues: []LeagueRef{
			{Source: "mystery_dm", SourceLeagueId: "47", Name: "L", Sport: "tennis"},
		},
	}
	logger, buf := newCaptureLog()
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	if lineContainsAll(buf, "999") {
		return
	}
	var persisted int64
	if err := db.Model(&events.Event{}).Where("league_id = ?", 999).Count(&persisted).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if persisted > 0 {
		return
	}
	t.Fatalf("expected either a log line mentioning league 999 OR the 999 match to be persisted; both fail\nlog:\n%s", buf.String())
}
