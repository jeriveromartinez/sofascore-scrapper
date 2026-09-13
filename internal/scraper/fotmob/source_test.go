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
		w.Write([]byte(`{"matches":{"allMatches":[{"id":"4193492","home":{"id":1,"name":"Chelsea","imageUrl":"https://x/c.png"},"away":{"id":2,"name":"Arsenal","imageUrl":"https://x/a.png"},"status":{"code":1,"type":"scheduled","finished":false,"started":false},"time":{"utcTime":"2026-09-10T20:00:00Z"}}]}}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL})
	src := NewSource(c, nil)
	league := scraper.LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League"}
	matches, err := src.ScheduledEvents(context.Background(), league, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("len: %d", len(matches))
	}
	m := matches[0]
	if m.SourceMatchId != "4193492" {
		t.Fatalf("SourceMatchId: %q", m.SourceMatchId)
	}
	if m.HomeTeam.Name != "Chelsea" {
		t.Fatalf("HomeTeam.Name: %q", m.HomeTeam.Name)
	}
}

func TestSource_SearchLeagues_FiltersLeagueSuggestions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("term"); got != "premier" {
			t.Errorf("query term: want premier, got %q", got)
		}
		w.Write([]byte(`{
			"suggestions": [
				{"type":"league","id":47,"name":"Premier League","country":"England","sport":"football"},
				{"type":"team","id":9825,"name":"Chelsea"},
				{"type":"league","id":48,"name":"EFL Championship","country":"England","sport":"football"}
			]
		}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL})
	src := NewSource(c, nil)
	results, err := src.SearchLeagues(context.Background(), "premier")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 league suggestions, got %d", len(results))
	}
	if results[0].SourceLeagueId != "47" || results[0].Name != "Premier League" {
		t.Errorf("first result: %+v", results[0])
	}
	if results[0].Source != "fotmob" {
		t.Errorf("source: want fotmob, got %q", results[0].Source)
	}
	if results[1].Country != "ENGLAND" {
		t.Errorf("country uppercase: want ENGLAND, got %q", results[1].Country)
	}
}

func TestSource_SearchLeagues_EmptyQueryReturnsEmpty(t *testing.T) {
	c := NewClient(ClientConfig{BaseURL: "http://unused"})
	src := NewSource(c, nil)
	results, err := src.SearchLeagues(context.Background(), "  ")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("want 0, got %d", len(results))
	}
}
