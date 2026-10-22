//go:build integration

package scraper

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
)

func TestService_ScrapeToday_RecordsScrapedEvents(t *testing.T) {
	const sport = "metrics-events-sport"
	repo := events.NewRepository(setupScraperTestDB(t))
	src := &dayMatchingFakeSource{
		name:    "metrics-events",
		matches: []Match{mkMatch("1", "47"), mkMatch("2", "47"), mkMatch("3", "87")},
	}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{
		{Source: "metrics-events", SourceLeagueId: "47", Name: "PL", Sport: sport},
		{Source: "metrics-events", SourceLeagueId: "87", Name: "LL", Sport: sport},
	}}
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	beforeEvents := gatheredSample(t, "scrape_events_total", map[string]string{"sport": sport})
	beforeOK := scrapeRequests(t, sport, "ok")

	svc.ScrapeToday(context.Background(), time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))

	if got := gatheredSample(t, "scrape_events_total", map[string]string{"sport": sport}) - beforeEvents; got != 3 {
		t.Errorf("scrape_events_total delta = %v, want 3", got)
	}
	if got := scrapeRequests(t, sport, "ok") - beforeOK; got != 2 {
		t.Errorf("scrape_requests_total{status=ok} delta = %v, want 2", got)
	}
}
