package events

import (
	"context"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
)

func TestServiceCountsCacheHitsAndMisses(t *testing.T) {
	svc, _, _ := newCachedService(t)
	hits := metricValue(t, observability.EventCacheHits)
	misses := metricValue(t, observability.EventCacheMisses)

	if _, err := svc.GetCurrentAndUpcoming(context.Background(), 1, 6); err != nil {
		t.Fatalf("first GetCurrentAndUpcoming: %v", err)
	}
	if got := metricValue(t, observability.EventCacheMisses) - misses; got != 1 {
		t.Fatalf("misses after cold request = %v, want 1", got)
	}
	if got := metricValue(t, observability.EventCacheHits) - hits; got != 0 {
		t.Fatalf("hits after cold request = %v, want 0", got)
	}

	if _, err := svc.GetCurrentAndUpcoming(context.Background(), 1, 6); err != nil {
		t.Fatalf("second GetCurrentAndUpcoming: %v", err)
	}
	if got := metricValue(t, observability.EventCacheHits) - hits; got != 1 {
		t.Fatalf("hits after warm request = %v, want 1", got)
	}
	if got := metricValue(t, observability.EventCacheMisses) - misses; got != 1 {
		t.Fatalf("misses after warm request = %v, want 1", got)
	}
}
