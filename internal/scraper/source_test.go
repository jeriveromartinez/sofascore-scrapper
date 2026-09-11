package scraper

import (
	"context"
	"testing"
	"time"
)

type fakeSource struct {
	scheduled []Match
	err       error
}

func (f *fakeSource) Name() string { return "fake" }
func (f *fakeSource) ScheduledEvents(ctx context.Context, league LeagueRef, date time.Time) ([]Match, error) {
	return f.scheduled, f.err
}
func (f *fakeSource) SearchLeagues(ctx context.Context, q string) ([]LeagueSearchResult, error) {
	return nil, nil
}

func TestSource_Contract(t *testing.T) {
	var _ Source = &fakeSource{}
}
