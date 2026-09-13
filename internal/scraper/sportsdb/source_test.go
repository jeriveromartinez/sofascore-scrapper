package sportsdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/sportsdb"
)

func TestSource_NameReturnsSportsDB(t *testing.T) {
	src := NewSource(sportsdb.NewClient(sportsdb.Options{}))
	if got := src.Name(); got != "sportsdb" {
		t.Errorf("Name() = %q, want sportsdb", got)
	}
}

func TestSource_ScheduledEventsDispatchesToClient(t *testing.T) {
	server := newFixtureServer(`{"events":[
		{"idEvent":"1","strHomeTeam":"Celtics","strAwayTeam":"Lakers","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","intHomeScore":"100","intAwayScore":"99"},
		{"idEvent":"2","strHomeTeam":"Heat","strAwayTeam":"Bulls","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","intHomeScore":"0","intAwayScore":"0"}
	]}`)
	defer server.Close()

	client := sportsdb.NewClient(sportsdb.Options{
		BaseURL: server.URL + "/api/v1/json/3",
	})
	src := NewSource(client)
	league := scraper.LeagueRef{
		Source: "sportsdb", SourceLeagueId: "4387",
		Name: "NBA", Country: "USA", Sport: "basketball",
	}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ScheduledEvents returned error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	if matches[0].HomeTeam.Name != "Celtics" || matches[0].AwayTeam.Name != "Lakers" {
		t.Errorf("match[0] = %v vs %v, want Celtics vs Lakers", matches[0].HomeTeam.Name, matches[0].AwayTeam.Name)
	}
	if matches[0].League.Sport != "basketball" {
		t.Errorf("match[0].League.Sport = %q, want basketball", matches[0].League.Sport)
	}
	if matches[0].HomeScore != 100 {
		t.Errorf("HomeScore = %d, want 100", matches[0].HomeScore)
	}
}

// TestSource_SkipsPostponedMatches ensures postponed matches are
// dropped before reaching the DB. TheSportsDB emits
// strPostponed:"yes" for rescheduled games; without this filter
// the daily upsert would overwrite a moved match with stale data.
func TestSource_SkipsPostponedMatches(t *testing.T) {
	server := newFixtureServer(`{"events":[
		{"idEvent":"1","strHomeTeam":"Celtics","strAwayTeam":"Lakers","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","intHomeScore":"100","intAwayScore":"99"},
		{"idEvent":"2","strHomeTeam":"Heat","strAwayTeam":"Bulls","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","strPostponed":"yes"}
	]}`)
	defer server.Close()

	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})
	src := NewSource(client)
	league := scraper.LeagueRef{
		Source: "sportsdb", SourceLeagueId: "4387",
		Name: "NBA", Country: "USA", Sport: "basketball",
	}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ScheduledEvents returned error: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("got %d matches, want 1 (postponed match must be dropped)", len(matches))
	}
	if matches[0].HomeTeam.Name != "Celtics" {
		t.Errorf("kept match = %q, want Celtics (the non-postponed one)", matches[0].HomeTeam.Name)
	}
}

// TestSource_SearchLeaguesReturns404Error documents that the
// search endpoint is intentionally not surfaced in this source
// — the scraper admin UI calls SearchLeagues to help operators
// discover FotMob league ids, and we keep sportsdb leagues
// curated via seed/admin form for the v1 multi-sport scope.
func TestSource_SearchLeaguesReturnsNotImplemented(t *testing.T) {
	src := NewSource(sportsdb.NewClient(sportsdb.Options{}))
	results, err := src.SearchLeagues(context.Background(), "NBA")
	if err == nil {
		t.Fatal("expected error from SearchLeagues (not implemented)")
	}
	if results != nil {
		t.Errorf("got %d results, want nil", len(results))
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error = %v, want one mentioning \"not supported\"", err)
	}
}

// newFixtureServer wires an httptest server that returns the
// supplied JSON payload verbatim regardless of path / query.
func newFixtureServer(payload string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
}
