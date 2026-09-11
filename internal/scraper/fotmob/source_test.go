package fotmob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

func TestSource_ScheduledEvents_MapsToMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PR #124: real /api/data/matches shape — id is int64,
		// team names under home.name/away.name, scores are ints on
		// the team objects (NOT a Status.ScoreStr string).
		_, _ = w.Write([]byte(`{
			"leagues": [
				{
					"id": 47,
					"name": "Premier League",
					"matches": [
						{
							"id": 4193492,
							"leagueId": 47,
							"home": {"id": 8455, "score": 2, "name": "Chelsea"},
							"away": {"id": 9825, "score": 1, "name": "Arsenal"},
							"statusId": 6,
							"status": {
								"utcTime": "2026-09-10T20:00:00Z",
								"finished": true,
								"started": true,
								"cancelled": false,
								"scoreStr": "2 - 1",
								"reason": {"short": "FT"}
							}
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	src := NewSource(c, nil)
	league := scraper.LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League"}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("len: %d", len(matches))
	}
	m := matches[0]
	// SourceMatchId comes from the int64 ID; the events repo
	// stores ExternalMatchId as a string and uses it as the unique
	// upsert key, so int64 must round-trip cleanly.
	if m.SourceMatchId != "4193492" {
		t.Fatalf("SourceMatchId: %q", m.SourceMatchId)
	}
	if m.HomeTeam.Name != "Chelsea" {
		t.Fatalf("HomeTeam.Name: %q", m.HomeTeam.Name)
	}
	if m.HomeScore != 2 {
		t.Errorf("HomeScore: want 2, got %d", m.HomeScore)
	}
	if m.AwayScore != 1 {
		t.Errorf("AwayScore: want 1, got %d", m.AwayScore)
	}
	if m.HomeTeam.SourceId != 8455 {
		t.Errorf("HomeTeam.SourceId: want 8455, got %d", m.HomeTeam.SourceId)
	}
	if m.Status.Type != "FT" {
		t.Errorf("Status.Type: want FT, got %q", m.Status.Type)
	}
	if m.Status.Started != true || m.Status.Finished != true {
		t.Errorf("Status: want started+finished true, got started=%v finished=%v", m.Status.Started, m.Status.Finished)
	}
	if !m.StartTimestamp.Equal(time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)) {
		t.Errorf("StartTimestamp: want 2026-09-10T20:00:00Z, got %v", m.StartTimestamp)
	}
}

// TestSource_ScheduledEvents_EmptyForOtherLeague ensures the
// Source drops matches belonging to other leagues that happen to
// come back in the same daily payload (PR #124).
func TestSource_ScheduledEvents_EmptyForOtherLeague(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns a LaLiga match for leagueId=87 but the
		// source requests league 47. The client must filter so
		// the source sees an empty day and the loop yields nothing.
		_, _ = w.Write([]byte(`{
			"leagues": [
				{
					"id": 87,
					"name": "LaLiga",
					"matches": [
						{
							"id": 5868059,
							"leagueId": 87,
							"home": {"id": 8302, "score": 0, "name": "Sevilla"},
							"away": {"id": 10267, "score": 0, "name": "Valencia"},
							"statusId": 0,
							"status": {"finished": false, "started": false, "scoreStr": ""}
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	src := NewSource(c, nil)
	league := scraper.LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League"}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("len: want 0 (other league filtered), got %d", len(matches))
	}
}
