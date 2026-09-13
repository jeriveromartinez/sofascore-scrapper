package sportsdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientAppliesDefaults(t *testing.T) {
	c := NewClient(Options{})
	if c.apiKey != DefaultAPIKey {
		t.Errorf("apiKey = %q, want %q", c.apiKey, DefaultAPIKey)
	}
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	if c.http == nil {
		t.Fatal("http client not initialized")
	}
	if c.http.Timeout != DefaultTimeout {
		t.Errorf("http timeout = %s, want %s", c.http.Timeout, DefaultTimeout)
	}
	if c.cache == nil {
		t.Fatal("cache map not initialized")
	}
}

func TestNewClientHonorsOptions(t *testing.T) {
	httpClient := &http.Client{Timeout: 2 * time.Second}
	c := NewClient(Options{
		APIKey:  "test-key",
		BaseURL: "https://example.test/v1/json",
		HTTP:    httpClient,
	})
	if c.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "test-key")
	}
	if c.baseURL != "https://example.test/v1/json" {
		t.Errorf("baseURL = %q, want override", c.baseURL)
	}
	if c.http != httpClient {
		t.Error("http client not honored")
	}
}

// TestTeamLogoURL_PrefersBadgeOverLogo documents that strBadge wins
// over strLogo because strBadge is the smaller badge variant, which
// is what TeamBadge expects to display.
func TestTeamLogoURL_PrefersBadgeOverLogo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "Juventus") {
			http.Error(w, "wrong query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResponse{
			Teams: []apiTeam{{
				IDTeam:   "133676",
				StrTeam:  "Juventus",
				StrBadge: "https://example.test/badge.png",
				StrLogo:  "https://example.test/logo.png",
			}},
		})
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	url, err := c.TeamLogoURL(context.Background(), "Juventus")
	if err != nil {
		t.Fatalf("TeamLogoURL returned error: %v", err)
	}
	if url != "https://example.test/badge.png" {
		t.Errorf("got %q, want badge URL", url)
	}
}

// TestTeamLogoURL_FallsBackToLogo covers the case where strBadge is
// missing but strLogo is present (older TheSportsDB entries).
func TestTeamLogoURL_FallsBackToLogo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(searchResponse{
			Teams: []apiTeam{{
				IDTeam:  "1",
				StrTeam: "Atletico Madrid",
				StrLogo: "https://example.test/atletico-logo.png",
			}},
		})
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	url, err := c.TeamLogoURL(context.Background(), "Atletico Madrid")
	if err != nil {
		t.Fatalf("TeamLogoURL returned error: %v", err)
	}
	if url != "https://example.test/atletico-logo.png" {
		t.Errorf("got %q, want logo URL fallback", url)
	}
}

// TestTeamLogoURL_CachesSuccessfulLookups ensures we only hit the
// upstream once per team name, so we stay well below TheSportsDB's
// public-key rate limit (~30 req/min).
func TestTeamLogoURL_CachesSuccessfulLookups(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(searchResponse{
			Teams: []apiTeam{{
				IDTeam:   "133676",
				StrTeam:  "Juventus",
				StrBadge: "https://example.test/juventus.png",
			}},
		})
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	for i := 0; i < 5; i++ {
		if _, err := c.TeamLogoURL(context.Background(), "Juventus"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 (cache should suppress repeats)", hits)
	}
}

// TestTeamLogoURL_NormalizesNameForCache documents that "Juventus",
// "juventus " and " JUVENTUS" all collapse onto the same cache slot
// so we don't ping TheSportsDB for trivial variations.
func TestTeamLogoURL_NormalizesNameForCache(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(searchResponse{
			Teams: []apiTeam{{
				IDTeam:   "133676",
				StrTeam:  "Juventus",
				StrBadge: "https://example.test/juventus.png",
			}},
		})
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	for _, name := range []string{"Juventus", "  juventus ", "JUVENTUS"} {
		if _, err := c.TeamLogoURL(context.Background(), name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 (name normalization should collapse cache slots)", hits)
	}
}

// TestTeamLogoURL_NoMatchReturnsError covers the not-found path so
// the LogoScheduler can decide whether to fall back to the next
// source.
func TestTeamLogoURL_NoMatchReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(searchResponse{Teams: nil})
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	url, err := c.TeamLogoURL(context.Background(), "Nonexistent FC")
	if err == nil {
		t.Fatal("expected error for unmatched team")
	}
	if url != "" {
		t.Errorf("expected empty URL on miss, got %q", url)
	}
	if !strings.Contains(err.Error(), "Nonexistent FC") {
		t.Errorf("error should mention team name, got %v", err)
	}
}

// TestTeamLogoURL_EmptyNameRejected documents that an empty name
// never reaches the upstream — saves a wasted API call.
func TestTeamLogoURL_EmptyNameRejected(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	for _, name := range []string{"", "   "} {
		if _, err := c.TeamLogoURL(context.Background(), name); err == nil {
			t.Errorf("%q: expected error", name)
		}
	}
	if hits != 0 {
		t.Errorf("upstream hits = %d, want 0 (empty names should not hit upstream)", hits)
	}
}

// TestTeamLogoURL_UpstreamErrorPropagates covers the failure path:
// the LogoScheduler must see the error so it can fall back to the
// SofaScore CDN URL.
func TestTeamLogoURL_UpstreamErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(Options{BaseURL: server.URL})
	_, err := c.TeamLogoURL(context.Background(), "Juventus")
	if err == nil {
		t.Fatal("expected error on upstream 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code, got %v", err)
	}
}

// TestTeamLogoURL_RateLimitStaggersConcurrentLookups ensures the
// shared rate limiter spaces successive upstream calls at least
// rateLimitInterval apart. Without it the 10-worker scheduler
// burst-trips TheSportsDB free-tier limit on the first run.
//
// We use a short rate limit window via NewClientForTest so the
// test runs in milliseconds, not seconds. The shape of the test
// mirrors the production behaviour: 5 concurrent calls produce
// exactly 5 hits spaced by the configured interval.
func TestTeamLogoURL_RateLimitStaggersConcurrentLookups(t *testing.T) {
	client := NewClientForTest(Options{}, 50*time.Millisecond)

	var hits int
	var timestamps []time.Time
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(searchResponse{
			Teams: []apiTeam{{
				IDTeam:   "1",
				StrTeam:  "X",
				StrBadge: "https://example.test/x.png",
			}},
		})
	}))
	defer server.Close()
	client.baseURL = server.URL

	names := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	var wg sync.WaitGroup
	for _, n := range names {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.TeamLogoURL(context.Background(), n)
		}()
	}
	wg.Wait()

	if hits != len(names) {
		t.Errorf("upstream hits = %d, want %d (each name must trigger a fetch the first time)", hits, len(names))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(timestamps) < 2 {
		return // single-hit case is trivially rate-limited
	}
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap+10*time.Millisecond < 50*time.Millisecond {
			t.Errorf("hits %d and %d were %s apart; want at least 50ms", i-1, i, gap)
		}
	}
}

// TestTeamLogoURL_RateLimitHonoursContext documents that a shutdown
// signal aborts the rate-limit wait instead of stalling the worker.
// This guards against a logo worker hanging for 2s on every call
// during graceful shutdown.
func TestTeamLogoURL_RateLimitHonoursContext(t *testing.T) {
	client := NewClientForTest(Options{}, 5*time.Second)
	// Force lastFetch to "now" so the next call wants to wait the
	// full 5s.
	client.lastFetch = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.waitForRateLimit(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestTeamLogoURL_429PausesUntilNextMinute documents that a
// TheSportsDB 429 response pauses subsequent calls for throttleBackoff
// (5 minutes by default). This is the production backoff after a
// throttle event and prevents the 10-worker scheduler from
// stampeding the API after the first 429.
func TestTeamLogoURL_429PausesUntilNextMinute(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "throttled", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL})
	// First call hits 429 and pauses the limiter for throttleBackoff.
	_, err := client.TeamLogoURL(context.Background(), "Juventus")
	if err == nil {
		t.Fatal("expected error from 429")
	}

	if remaining := client.throttleRemaining(); remaining <= 0 {
		t.Errorf("expected throttleRemaining > 0 after 429, got %v", remaining)
	}
	if remaining := client.throttleRemaining(); remaining < throttleBackoff-time.Second {
		t.Errorf("throttleRemaining = %v, want >= %v", remaining, throttleBackoff-time.Second)
	}
}

// TestTeamLogoURL_429SecondCallWaits verifies that the second call
// after a 429 actually sleeps for the throttle remainder rather
// than proceeding immediately. This is the integration of the
// recordThrottle + waitForRateLimit pair and the one that
// regressed in production — recordThrottle set the timer but
// waitForRateLimit ignored it.
func TestTeamLogoURL_429SecondCallWaits(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "throttled", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL})
	if _, err := client.TeamLogoURL(context.Background(), "Juventus"); err == nil {
		t.Fatal("expected error from first 429")
	}

	// Manually set throttleUntil to 800ms in the future and verify
	// the second call waits that long. This sidesteps the
	// wall-clock-minute boundary so the test runs in under 1s.
	client.throttleMu.Lock()
	client.throttleUntil = time.Now().Add(800 * time.Millisecond)
	client.throttleMu.Unlock()

	start := time.Now()
	if _, err := client.TeamLogoURL(context.Background(), "Inter"); err == nil {
		t.Fatal("expected error from second 429")
	}
	elapsed := time.Since(start)
	if elapsed < 700*time.Millisecond {
		t.Errorf("second call elapsed = %s, want >= 700ms (throttle not honored)", elapsed)
	}
}

// TestTeamLogoURL_429CachedAsNegativeHit documents that the 429
// error is cached so the scheduler does not hammer the upstream
// with the same failing team on every worker. The negative cache
// TTL (15 min by default) ensures we retry eventually.
func TestTeamLogoURL_429CachedAsNegativeHit(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "throttled", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL})
	for i := 0; i < 5; i++ {
		_, _ = client.TeamLogoURL(context.Background(), "Juventus")
	}
	// First call passes through to upstream; subsequent calls hit
	// the negative cache and never reach upstream. We expect at
	// most 1 upstream hit (subsequent calls all read the negative
	// cache before waitForRateLimit, so they don't even consult the
	// throttled server).
	if got := atomic.LoadInt32(&hits); got > 1 {
		t.Errorf("upstream hits = %d, want <= 1 (negative cache must suppress retries)", got)
	}
}
