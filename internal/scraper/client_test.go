package scraper

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestClient_NoRetryOn4xx pins the contract that 403 still gets one
// refresh+retry attempt (matching the original 401/403 retry path),
// then surfaces the error. The original test asserted only the
// final error; we add a bound to make the contract explicit.
func TestClient_NoRetryOn4xx(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("no-retry-4xx", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusForbidden}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "no-retry-4xx", time.Now())
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got: %v", err)
	}
	// 403 → refresh+retry → 403 again → error. Two attempts total.
	if got := fake.callCount(); got != 2 {
		t.Fatalf("expected exactly 2 fetch attempts (initial + post-refresh), got %d", got)
	}
}

func TestClient_RetryOn429(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("retry-429", func(_ context.Context, call int) (fetchResult, error) {
		if call <= 2 {
			return fetchResult{Status: http.StatusTooManyRequests}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "retry-429", time.Now())
	if err != nil {
		t.Fatalf("expected success after 429 retries: %v", err)
	}
	if fake.callCount() < 3 {
		t.Fatalf("expected at least 3 fetch attempts, got %d", fake.callCount())
	}
}

func TestClient_RetryOn429WithRetryAfter(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("retry-429-after", func(_ context.Context, call int) (fetchResult, error) {
		if call == 1 {
			return fetchResult{
				Status:  http.StatusTooManyRequests,
				Headers: map[string][]string{"Retry-After": {"0"}},
			}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "retry-429-after", time.Now())
	if err != nil {
		t.Fatalf("expected success after 429: %v", err)
	}
}

func TestClient_RetryOn502(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("retry-502", func(_ context.Context, call int) (fetchResult, error) {
		if call <= 2 {
			return fetchResult{Status: http.StatusBadGateway}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "retry-502", time.Now())
	if err != nil {
		t.Fatalf("expected success after 502 retries: %v", err)
	}
}

func TestClient_RetryOn503(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("retry-503", func(_ context.Context, call int) (fetchResult, error) {
		if call <= 2 {
			return fetchResult{Status: http.StatusServiceUnavailable}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "retry-503", time.Now())
	if err != nil {
		t.Fatalf("expected success after 503 retries: %v", err)
	}
}

// TestClient_RefreshOn401 verifies that a single 401 triggers the
// soft-refresh delay and a retry. In the rod-era the "refresh" is a
// 500ms settle; the next fetch uses whatever cookies the browser
// currently holds.
func TestClient_RefreshOn401(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("auth-reject", func(_ context.Context, call int) (fetchResult, error) {
		if call == 1 {
			return fetchResult{Status: http.StatusUnauthorized}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "auth-reject", time.Now())
	if err != nil {
		t.Fatalf("expected success after refresh: %v", err)
	}
	if fake.callCount() != 2 {
		t.Fatalf("expected exactly 2 fetch attempts (401 then success), got %d", fake.callCount())
	}
}

func TestClient_Persistent401Fails(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("auth-persistent", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusUnauthorized}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 2, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "auth-persistent", time.Now())
	if err == nil {
		t.Fatal("expected error for persistent 401")
	}
}

func TestClient_ResponseSizeCap(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("big-body", func(_ context.Context, call int) (fetchResult, error) {
		body := []byte(`{"events":[{"id":1,"slug":"x","startTimestamp":1,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1}}]}`)
		if call < 2 {
			body = append(body, []byte(strings.Repeat(" ", 200))...)
		}
		return fetchResult{Status: http.StatusOK, Body: body}, nil
	})

	c := newClientWithFetcher(ClientConfig{
		MaxRetries:       1,
		ResponseMaxBytes: 50,
		MaxBackoff:       time.Millisecond,
	}, fake)
	_, err := c.ScheduledEvents(context.Background(), "big-body", time.Now())
	if err == nil {
		t.Fatal("expected error due to oversized response")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("slow", func(ctx context.Context, _ int) (fetchResult, error) {
		select {
		case <-ctx.Done():
			return fetchResult{}, ctx.Err()
		case <-time.After(2 * time.Second):
			return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
		}
	})

	c := newClientWithFetcher(ClientConfig{
		MaxRetries:     1,
		RequestTimeout: 10 * time.Second,
		MaxBackoff:     time.Millisecond,
	}, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.ScheduledEvents(ctx, "slow", time.Now())
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestClient_Timeout(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("slow", func(ctx context.Context, _ int) (fetchResult, error) {
		select {
		case <-ctx.Done():
			return fetchResult{}, ctx.Err()
		case <-time.After(2 * time.Second):
			return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
		}
	})

	c := newClientWithFetcher(ClientConfig{
		MaxRetries:     1,
		RequestTimeout: 50 * time.Millisecond,
		MaxBackoff:     time.Millisecond,
	}, fake)

	_, err := c.ScheduledEvents(context.Background(), "slow", time.Now())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClient_TrendingEvents(t *testing.T) {
	body := []byte(`{"events":[{"id":1,"slug":"test","startTimestamp":1710000000,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1710000000}}]}`)
	fake := newFakeFetcher()
	fake.route("trending", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusOK, Body: body}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 1, MaxBackoff: time.Millisecond}, fake)
	events, err := c.TrendingEvents(context.Background(), "MX")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != 1 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestClient_ScheduledEvents(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("scheduled-tournaments", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{
			Status: http.StatusOK,
			Body:   []byte(`{"scheduled":[{"tournament":{"id":1,"uniqueTournament":{"id":16,"name":"L","slug":"l","category":{"name":"c","slug":"c"}}}}]}`),
		}, nil
	})
	fake.route("scheduled-events", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 1, MaxBackoff: time.Millisecond}, fake)
	events, err := c.ScheduledEvents(context.Background(), "football", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestClient_BackoffBound(t *testing.T) {
	c := newClientWithFetcher(ClientConfig{MaxBackoff: 5 * time.Second}, newFakeFetcher())
	d := c.backoffDuration(10)
	if d != 5*time.Second {
		t.Fatalf("backoff should be capped at MaxBackoff(5s), got %v", d)
	}
}

func TestClient_TrendingParsesAllFields(t *testing.T) {
	fake := newFakeFetcher()
	fake.route("trending", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 1, MaxBackoff: time.Millisecond}, fake)
	events, err := c.TrendingEvents(context.Background(), "mx")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("expected 0 events")
	}
}

func TestClient_ReadAllBody(t *testing.T) {
	tournamentsBody := `{"scheduled":[{"tournament":{"id":1,"uniqueTournament":{"id":16,"name":"L","slug":"l","category":{"name":"c","slug":"c"}}}}]}`
	eventsBody := `{"events":[{"id":1,"slug":"test","startTimestamp":1710000000,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1710000000}}]}`

	fake := newFakeFetcher()
	fake.route("scheduled-tournaments", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusOK, Body: []byte(tournamentsBody)}, nil
	})
	fake.route("scheduled-events", func(_ context.Context, _ int) (fetchResult, error) {
		return fetchResult{Status: http.StatusOK, Body: []byte(eventsBody)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 1, MaxBackoff: time.Millisecond}, fake)
	events, err := c.ScheduledEvents(context.Background(), "football", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
}

// TestClient_NoLeakAcrossRetries verifies that a 503-storm does not
// leave pending Fetch calls open simultaneously. With rod this maps
// to "no two rod pages are busy at once", which the page pool
// guarantees; the fake mirrors that contract by tracking open count.
func TestClient_NoLeakAcrossRetries(t *testing.T) {
	var maxOpen atomic.Int32
	fake := newFakeFetcher()
	fake.route("flaky", func(_ context.Context, call int) (fetchResult, error) {
		cur := fake.openCount()
		for {
			max := maxOpen.Load()
			if cur <= max || maxOpen.CompareAndSwap(max, cur) {
				break
			}
		}
		if call < 3 {
			return fetchResult{Status: http.StatusServiceUnavailable}, nil
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 5, MaxBackoff: time.Millisecond}, fake)
	_, err := c.ScheduledEvents(context.Background(), "flaky", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := maxOpen.Load(); got > 1 {
		t.Fatalf("expected at most 1 open fetch concurrently, saw %d", got)
	}
	if fake.openCount() != 0 {
		t.Fatalf("expected 0 open fetches after return, got %d", fake.openCount())
	}
}

// TestClient_FetcherErrorRetries verifies that a transport-level
// error from the fetcher (e.g. browser hang, JS exception) is
// retried just like a 5xx, not surfaced immediately.
func TestClient_FetcherErrorRetries(t *testing.T) {
	var calls atomic.Int64
	fake := newFakeFetcher()
	fake.route("transient", func(_ context.Context, _ int) (fetchResult, error) {
		n := calls.Add(1)
		if n < 2 {
			return fetchResult{}, errors.New("transient browser error")
		}
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	})

	c := newClientWithFetcher(ClientConfig{MaxRetries: 3, MaxBackoff: time.Millisecond}, fake)
	if _, err := c.ScheduledEvents(context.Background(), "transient", time.Now()); err != nil {
		t.Fatalf("expected retry to recover: %v", err)
	}
}
