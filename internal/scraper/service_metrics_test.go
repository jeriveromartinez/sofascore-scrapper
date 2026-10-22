package scraper

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
)

// metricsDayMatcher is a bulk-fetch source whose day fetch fails.
type metricsDayMatcher struct {
	failingSource
	dayErr error
}

func (s *metricsDayMatcher) DayMatches(ctx context.Context, date time.Time) ([]Match, error) {
	return nil, s.dayErr
}

// gatheredSample returns the counter value or histogram sample count of
// the series of metric name whose labels include want; 0 when the
// series does not exist yet. prometheus/testutil is not used because
// it pulls modules the go.mod does not list.
func gatheredSample(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()
	families, err := observability.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			matched := 0
			for _, lp := range m.GetLabel() {
				if v, ok := want[lp.GetName()]; ok && v == lp.GetValue() {
					matched++
				}
			}
			if matched != len(want) {
				continue
			}
			if h := m.GetHistogram(); h != nil {
				return float64(h.GetSampleCount())
			}
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

func scrapeRequests(t *testing.T, sport, status string) float64 {
	return gatheredSample(t, "scrape_requests_total", map[string]string{"sport": sport, "status": status})
}

func scrapeDurationSamples(t *testing.T, sport string) float64 {
	return gatheredSample(t, "scrape_duration_seconds", map[string]string{"sport": sport})
}

func runScrapeToday(t *testing.T, src Source, leagues []LeagueRef) {
	t.Helper()
	svc, err := NewService(nil, NewSourceDispatcher(src), &staticCatalog{leagues: leagues}, 10, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.ScrapeToday(context.Background(), time.Now())
}

// The scrape_requests_total / scrape_duration_seconds metrics were
// registered but never written, so the scrape failure-rate alert had
// no data to evaluate.
func TestScrapeToday_RecordsSuccessfulLeagueScrape(t *testing.T) {
	const sport = "metrics-ok-sport"
	beforeOK, beforeErr := scrapeRequests(t, sport, "ok"), scrapeRequests(t, sport, "error")
	beforeSamples := scrapeDurationSamples(t, sport)

	runScrapeToday(t, &failingSource{name: "metrics-ok"}, []LeagueRef{
		{Source: "metrics-ok", SourceLeagueId: "1", Sport: sport},
	})

	if got := scrapeRequests(t, sport, "ok") - beforeOK; got != 1 {
		t.Errorf("scrape_requests_total{status=ok} delta = %v, want 1", got)
	}
	if got := scrapeRequests(t, sport, "error") - beforeErr; got != 0 {
		t.Errorf("scrape_requests_total{status=error} delta = %v, want 0", got)
	}
	if got := scrapeDurationSamples(t, sport) - beforeSamples; got != 1 {
		t.Errorf("scrape_duration_seconds samples delta = %v, want 1", got)
	}
}

func TestScrapeToday_RecordsFailedLeagueScrape(t *testing.T) {
	const sport = "metrics-error-sport"
	beforeOK, beforeErr := scrapeRequests(t, sport, "ok"), scrapeRequests(t, sport, "error")

	runScrapeToday(t, &failingSource{name: "metrics-error", err: errors.New("upstream is down")}, []LeagueRef{
		{Source: "metrics-error", SourceLeagueId: "1", Sport: sport},
		{Source: "metrics-error", SourceLeagueId: "2", Sport: sport},
	})

	if got := scrapeRequests(t, sport, "error") - beforeErr; got != 2 {
		t.Errorf("scrape_requests_total{status=error} delta = %v, want 2", got)
	}
	if got := scrapeRequests(t, sport, "ok") - beforeOK; got != 0 {
		t.Errorf("scrape_requests_total{status=ok} delta = %v, want 0", got)
	}
}

func TestScrapeToday_RecordsFailedDayFetchForEveryLeague(t *testing.T) {
	const sport = "metrics-day-error-sport"
	before := scrapeRequests(t, sport, "error")
	src := &metricsDayMatcher{failingSource: failingSource{name: "metrics-day"}, dayErr: errors.New("upstream is down")}

	runScrapeToday(t, src, []LeagueRef{
		{Source: "metrics-day", SourceLeagueId: "1", Sport: sport},
		{Source: "metrics-day", SourceLeagueId: "2", Sport: sport},
		{Source: "metrics-day", SourceLeagueId: "3", Sport: sport},
	})

	if got := scrapeRequests(t, sport, "error") - before; got != 3 {
		t.Errorf("scrape_requests_total{status=error} delta = %v, want 3", got)
	}
}
