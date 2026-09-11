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
