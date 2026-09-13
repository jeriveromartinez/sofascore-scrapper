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

// TestSource_PopulatesTeamIDsAndLogos covers the team-link fix
// from PR #132: eventsday.php returns idHomeTeam/idAwayTeam and
// strHomeTeamBadge/strAwayTeamBadge, and toMatch must wire them
// into scraper.Match so the events table gets FK links to a row
// in the teams table with a downloadable logo URL.
//
// Regression guard: if toMatch stops forwarding the new fields,
// the events list will show empty teams for every TheSportsDB
// sport (NBA/NFL/MLB/NHL) and this test will fail.
func TestSource_PopulatesTeamIDsAndLogos(t *testing.T) {
	server := newFixtureServer(`{"events":[
		{"idEvent":"1","strHomeTeam":"Cincinnati Bengals","strAwayTeam":"Tampa Bay Buccaneers","dateEvent":"2026-09-13","strTimestamp":"2026-09-13T17:00:00","strLeague":"NFL","strSport":"American Football","intHomeScore":"33","intAwayScore":"27","idHomeTeam":"134923","idAwayTeam":"134945","strHomeTeamBadge":"https://r2.thesportsdb.com/images/media/team/badge/h1ce8y1784717263.png","strAwayTeamBadge":"https://r2.thesportsdb.com/images/media/team/badge/2dfpdl1537820969.png"}
	]}`)
	defer server.Close()

	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})
	src := NewSource(client)
	league := scraper.LeagueRef{
		Source: "sportsdb", SourceLeagueId: "4391",
		Name: "NFL", Country: "USA", Sport: "american-football",
	}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ScheduledEvents returned error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	m := matches[0]

	// Team IDs must be prefixed with TeamIDPrefix so they don't
	// collide with FotMob-sourced teams in the shared table.
	wantHome := int64(134923) + TeamIDPrefix
	wantAway := int64(134945) + TeamIDPrefix
	if m.HomeTeam.SourceId != wantHome {
		t.Errorf("HomeTeam.SourceId = %d, want %d (prefix %d + id 134923)", m.HomeTeam.SourceId, wantHome, TeamIDPrefix)
	}
	if m.AwayTeam.SourceId != wantAway {
		t.Errorf("AwayTeam.SourceId = %d, want %d (prefix %d + id 134945)", m.AwayTeam.SourceId, wantAway, TeamIDPrefix)
	}

	// Logo URLs come from the upstream payload (strHomeTeamBadge),
	// not from the SofaScore CDN fallback, so the LogoScheduler
	// downloads them directly.
	if m.HomeTeam.LogoURL != "https://r2.thesportsdb.com/images/media/team/badge/h1ce8y1784717263.png" {
		t.Errorf("HomeTeam.LogoURL = %q, want the upstream badge URL", m.HomeTeam.LogoURL)
	}
	if m.AwayTeam.LogoURL != "https://r2.thesportsdb.com/images/media/team/badge/2dfpdl1537820969.png" {
		t.Errorf("AwayTeam.LogoURL = %q, want the upstream badge URL", m.AwayTeam.LogoURL)
	}
}

// TestSource_PrefixedTeamIDsDoNotCollideWithFotMob pins the
// namespacing invariant: a TheSportsDB ID and a FotMob ID that
// happen to share the same numeric value must end up as distinct
// team_id rows in the shared teams table.
func TestSource_PrefixedTeamIDsDoNotCollideWithFotMob(t *testing.T) {
	const upstreamID = int64(9885) // Juventus on FotMob, hypothetical TheSportsDB club
	prefixed := upstreamID + TeamIDPrefix
	if prefixed <= 1_701_119 {
		t.Fatalf("TeamIDPrefix = %d; namespace must exceed observed FotMob max (~1.7M) so collisions stay impossible. prefixed=%d would collide", TeamIDPrefix, prefixed)
	}
}

// TestSource_DayMatches_ReturnsAllLeagues locks in the bulk-fetch
// path: one HTTP call yields matches across multiple leagues.
// Without this the source would fall back to per-league fan-out
// and burn the free-tier rate limit on the same calendar day.
func TestSource_DayMatches_ReturnsAllLeagues(t *testing.T) {
	server := newFixtureServer(`{"events":[
		{"idEvent":"1","idLeague":"4387","strHomeTeam":"Lakers","strAwayTeam":"Celtics","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","strCountry":"USA","intHomeScore":"100","intAwayScore":"99","idHomeTeam":"1","idAwayTeam":"2","strPostponed":"no"},
		{"idEvent":"2","idLeague":"4391","strHomeTeam":"Bengals","strAwayTeam":"Buccaneers","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T17:00:00","strLeague":"NFL","strSport":"American Football","strCountry":"USA","intHomeScore":"33","intAwayScore":"27","idHomeTeam":"3","idAwayTeam":"4","strPostponed":"no"}
	]}`)
	defer server.Close()

	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})
	src := NewSource(client)
	matches, err := src.DayMatches(context.Background(), time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2 (one per league)", len(matches))
	}
	// Each match's League.SourceLeagueId must carry the event's
	// idLeague so the dispatcher can bucket by league.
	if matches[0].League.SourceLeagueId != "4387" || matches[0].League.Sport != "basketball" {
		t.Errorf("matches[0].League = %+v, want NBA / basketball", matches[0].League)
	}
	if matches[1].League.SourceLeagueId != "4391" || matches[1].League.Sport != "american-football" {
		t.Errorf("matches[1].League = %+v, want NFL / american-football", matches[1].League)
	}
}

// TestSource_DayMatches_DropsPostponedAndUnbound covers the two
// invariants the day-wide path relies on: postponed matches are
// skipped (so a rescheduled fixture doesn't overwrite fresh data)
// and events without an idLeague binding are dropped (so we
// never persist a synthetic league).
func TestSource_DayMatches_DropsPostponedAndUnbound(t *testing.T) {
	server := newFixtureServer(`{"events":[
		{"idEvent":"1","idLeague":"4387","strHomeTeam":"Lakers","strAwayTeam":"Celtics","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","intHomeScore":"100","intAwayScore":"99","strPostponed":"no"},
		{"idEvent":"2","idLeague":"4387","strHomeTeam":"Heat","strAwayTeam":"Bulls","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strLeague":"NBA","strSport":"Basketball","strPostponed":"yes"},
		{"idEvent":"3","strHomeTeam":"NoLeague","strAwayTeam":"Bound","dateEvent":"2026-01-15","strTimestamp":"2026-01-15T00:00:00","strSport":"Unknown","intHomeScore":"0","intAwayScore":"0","strPostponed":"no"}
	]}`)
	defer server.Close()

	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})
	src := NewSource(client)
	matches, err := src.DayMatches(context.Background(), time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1 (postponed + no-league must both be dropped)", len(matches))
	}
	if matches[0].HomeTeam.Name != "Lakers" {
		t.Errorf("kept match = %q, want Lakers", matches[0].HomeTeam.Name)
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
