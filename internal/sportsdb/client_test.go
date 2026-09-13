package sportsdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

// TestEventsByDay_ParsesRealShape exercises the TheSportsDB
// eventsday.php payload shape we observed against the live API
// (NBA on 2026-01-15). The fixture mirrors the documented fields
// exactly so a payload-format drift trips the test.
func TestEventsByDay_ParsesRealShape(t *testing.T) {
	fixture := `{"events":[
		{
			"idEvent":"2357930",
			"idAPIfootball":"470046",
			"strTimestamp":"2026-01-15T00:00:00",
			"strEvent":"Indiana Pacers vs Toronto Raptors",
			"strHomeTeam":"Indiana Pacers",
			"strAwayTeam":"Toronto Raptors",
			"intHomeScore":"101",
			"intAwayScore":"115",
			"dateEvent":"2026-01-15",
			"strTime":"00:00:00",
			"strTimeLocal":"19:00:00",
			"idHomeTeam":"134873",
			"idAwayTeam":"134864",
			"strLeague":"NBA",
			"strSport":"Basketball",
			"strStatus":"Not Started",
			"strPostponed":"no"
		}
	]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "l=4387") {
			http.Error(w, "wrong league", http.StatusBadRequest)
			return
		}
		if !strings.Contains(r.URL.RawQuery, "d=2026-01-15") {
			http.Error(w, "wrong date", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	events, err := client.EventsByDay(context.Background(), "4387", "2026-01-15")
	if err != nil {
		t.Fatalf("EventsByDay returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	if got.IDEvent != "2357930" {
		t.Errorf("IDEvent = %q, want 2357930", got.IDEvent)
	}
	if got.HomeTeam != "Indiana Pacers" {
		t.Errorf("HomeTeam = %q, want Indiana Pacers", got.HomeTeam)
	}
	if got.AwayTeam != "Toronto Raptors" {
		t.Errorf("AwayTeam = %q, want Toronto Raptors", got.AwayTeam)
	}
	if got.HomeScore != 101 {
		t.Errorf("HomeScore = %d, want 101", got.HomeScore)
	}
	if got.AwayScore != 115 {
		t.Errorf("AwayScore = %d, want 115", got.AwayScore)
	}
	if got.League != "NBA" {
		t.Errorf("League = %q, want NBA", got.League)
	}
	if got.Sport != "Basketball" {
		t.Errorf("Sport = %q, want Basketball", got.Sport)
	}
	if got.Timestamp.IsZero() {
		t.Errorf("Timestamp is zero; expected 2026-01-15T00:00:00")
	}
	if got.Postponed {
		t.Errorf("Postponed = true, want false (not postponed)")
	}
}

// TestEventsByDay_EmptyDayReturnsEmptySlice covers the documented
// "no events today" path: TheSportsDB returns {"events":null}
// (literal null, not []) for an empty day. The client must treat
// both shapes as an empty result.
func TestEventsByDay_EmptyDayReturnsEmptySlice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":null}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	events, err := client.EventsByDay(context.Background(), "4387", "2026-01-15")
	if err != nil {
		t.Fatalf("EventsByDay returned error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}

// TestEventsByDay_RateLimited ensures the shared rate limiter
// spaces subsequent calls. Without it, the multi-sport scrape
// loop would burst 8 simultaneous TheSportsDB hits and trip
// the 30 req/min free-tier quota. The callers vary (leagueID,
// date) per call so the in-memory cache cannot short-circuit
// any of them — every request goes through the rate limiter.
func TestEventsByDay_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":[]}`)
	}))
	defer server.Close()

	client := NewClientForTest(Options{BaseURL: server.URL + "/api/v1/json/3"}, 80*time.Millisecond)

	// Each call uses a distinct (leagueID, date) so the cache
	// cannot absorb a repeat — every request reaches the rate
	// limiter. The 80ms interval × 3 calls = ≥160ms expected.
	calls := []struct {
		league, date string
	}{
		{"4387", "2026-01-15"},
		{"4387", "2026-01-16"},
		{"4387", "2026-01-17"},
	}
	start := time.Now()
	for _, call := range calls {
		if _, err := client.EventsByDay(context.Background(), call.league, call.date); err != nil {
			t.Fatalf("EventsByDay(%s, %s): %v", call.league, call.date, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Errorf("3 calls elapsed = %s, want >= 150ms (rate limit must space them)", elapsed)
	}
}

// TestEventsByDay_429PausesUntilWindowExpires verifies that a 429
// response pauses subsequent calls. TheSportsDB's free tier
// throttles at ~30 req/min; without this backoff the
// multi-sport scraper would trip it on the first league burst.
func TestEventsByDay_429PausesUntilWindowExpires(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "throttled", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	if _, err := client.EventsByDay(context.Background(), "4387", "2026-01-15"); err == nil {
		t.Fatal("expected error from 429")
	}
	if remaining := client.throttleRemaining(); remaining <= 0 {
		t.Errorf("expected throttleRemaining > 0 after 429, got %v", remaining)
	}
}

// TestEventsByDay_PostponedFlagDocumentsShape documents the
// "strPostponed":"yes" shape the upstream emits when a match is
// postponed. The scraper marks the result as cancelled and skips
// it from the daily upsert; the test pins the field name so a
// upstream payload rename is caught.
func TestEventsByDay_PostponedFlagDocumentsShape(t *testing.T) {
	fixture := `{"events":[{
		"idEvent":"9999",
		"strEvent":"PSG vs Marseille",
		"strHomeTeam":"PSG",
		"strAwayTeam":"Marseille",
		"dateEvent":"2026-01-15",
		"strTime":"20:00:00",
		"idHomeTeam":"1",
		"idAwayTeam":"2",
		"strLeague":"Ligue 1",
		"strSport":"Soccer",
		"strStatus":"Postponed",
		"strPostponed":"yes"
	}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	events, err := client.EventsByDay(context.Background(), "4335", "2026-01-15")
	if err != nil {
		t.Fatalf("EventsByDay returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].Postponed {
		t.Errorf("Postponed = false, want true (strPostponed=yes)")
	}
}

// TestEventsByDay_CachesSuccess verifies successful lookups are
// cached so the daily cron does not re-hit TheSportsDB for the
// same (league, date) within cacheTTL.
func TestEventsByDay_CachesSuccess(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":[]}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	for i := 0; i < 5; i++ {
		if _, err := client.EventsByDay(context.Background(), "4387", "2026-01-15"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (cache should suppress repeats)", got)
	}
}

// TestAllLeagues_ParsesRealShape locks in the wire shape of
// all_leagues.php. The free tier returns ~10 entries per request,
// so a fixture with the truncated sample is enough.
func TestAllLeagues_ParsesRealShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"leagues":[
			{"idLeague":"4328","strLeague":"English Premier League","strSport":"Soccer","strCountry":"England"},
			{"idLeague":"4387","strLeague":"NBA","strSport":"Basketball","strCountry":"USA"},
			{"idLeague":"4391","strLeague":"NFL","strSport":"American Football","strCountry":"USA"}
		]}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	leagues, err := client.AllLeagues(context.Background())
	if err != nil {
		t.Fatalf("AllLeagues: %v", err)
	}
	if len(leagues) != 3 {
		t.Fatalf("got %d leagues, want 3", len(leagues))
	}
	if leagues[1].ID != "4387" || leagues[1].Name != "NBA" || leagues[1].Sport != "Basketball" {
		t.Errorf("leagues[1] = %+v, want NBA/Basketball", leagues[1])
	}
	if leagues[2].Country != "USA" {
		t.Errorf("leagues[2].Country = %q, want USA", leagues[2].Country)
	}
}

// TestAllLeagues_CachesSuccessfulLookups verifies the in-memory
// cache suppresses repeats — same as the logo-lookup path so a
// daily cron does not hammer the upstream for an unchanged list.
func TestAllLeagues_CachesSuccessfulLookups(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"leagues":[{"idLeague":"4328","strLeague":"EPL","strSport":"Soccer","strCountry":"England"}]}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	for i := 0; i < 3; i++ {
		if _, err := client.AllLeagues(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1", got)
	}
}

// TestAllLeagues_429CachedAsNegativeHit pins the throttle contract:
// a 429 response is cached briefly (15min) so a tight loop does
// not spam the upstream while the throttle window is still active.
func TestAllLeagues_429CachedAsNegativeHit(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	for i := 0; i < 3; i++ {
		if _, err := client.AllLeagues(context.Background()); err == nil {
			t.Fatalf("call %d: expected 429 error", i)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (429 must be cached as negative hit)", got)
	}
}

// TestNormalizeSport covers the canonical mapping for every sport
// the multi-sport scraper seeds today and a fallback for unknown
// sports so the catalog never sees an upper-case raw string.
func TestNormalizeSport(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Soccer", "football"},
		{"soccer", "football"},
		{"American Football", "american-football"},
		{"Ice Hockey", "ice-hockey"},
		{"Basketball", "basketball"},
		{"Baseball", "baseball"},
		{"", ""},
		{"   ", ""},
		{"Cricket", "cricket"},
		{"Motor Sport", "motor-sport"},
		{"  Rugby Union  ", "rugby-union"},
	}
	for _, c := range cases {
		if got := NormalizeSport(c.in); got != c.want {
			t.Errorf("NormalizeSport(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEventsDayAll_ParsesRealShape covers the day-wide path:
// eventsday.php?d=<date> with no league filter returns events
// across every sport the upstream publishes for that date.
// Sample fixture mixes NFL (American Football) and NBA
// (Basketball) to lock in the multi-sport decoding.
func TestEventsDayAll_ParsesRealShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "d=2026-09-13") {
			http.Error(w, "wrong date", http.StatusBadRequest)
			return
		}
		if strings.Contains(r.URL.RawQuery, "l=") {
			http.Error(w, "must not include league filter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":[
			{"idEvent":"2475376","idLeague":"4391","strHomeTeam":"Cincinnati Bengals","strAwayTeam":"Tampa Bay Buccaneers","dateEvent":"2026-09-13","strTimestamp":"2026-09-13T17:00:00","strLeague":"NFL","strSport":"American Football","strCountry":"United States","intHomeScore":"33","intAwayScore":"27","idHomeTeam":"134923","idAwayTeam":"134945","strHomeTeamBadge":"https://x/y.png","strAwayTeamBadge":"https://x/z.png","strPostponed":"no"},
			{"idEvent":"441613","idLeague":"4387","strHomeTeam":"Los Angeles Lakers","strAwayTeam":"Boston Celtics","dateEvent":"2026-09-13","strTimestamp":"2026-09-13T00:00:00","strLeague":"NBA","strSport":"Basketball","strCountry":"USA","intHomeScore":"0","intAwayScore":"0","idHomeTeam":"133604","idAwayTeam":"133601","strHomeTeamBadge":"https://x/a.png","strAwayTeamBadge":"https://x/b.png","strPostponed":"no"}
		]}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	events, err := client.EventsDayAll(context.Background(), "2026-09-13")
	if err != nil {
		t.Fatalf("EventsDayAll: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].IDLeague != "4391" || events[0].League != "NFL" || events[0].Sport != "American Football" {
		t.Errorf("events[0] league = (%q, %q, %q), want (4391, NFL, American Football)",
			events[0].IDLeague, events[0].League, events[0].Sport)
	}
	if events[1].IDLeague != "4387" || events[1].Sport != "Basketball" {
		t.Errorf("events[1] league = (%q, %q), want (4387, Basketball)", events[1].IDLeague, events[1].Sport)
	}
	if events[0].Country != "United States" {
		t.Errorf("events[0].Country = %q, want United States", events[0].Country)
	}
}

// TestEventsDayAll_CachesSuccess ensures the day-wide fetch is
// cached for 24h on success — same contract as the per-league
// path so a daily cron does not re-hit a stable day.
func TestEventsDayAll_CachesSuccess(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":[]}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	for i := 0; i < 3; i++ {
		if _, err := client.EventsDayAll(context.Background(), "2026-09-13"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1", got)
	}
}

// TestEventsDayAll_EmptyDayReturnsEmptySlice covers the documented
// shape quirk: TheSportsDB returns {"events":null} on empty days.
// The client surfaces a non-nil empty slice so callers don't have
// to nil-check.
func TestEventsDayAll_EmptyDayReturnsEmptySlice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"events":null}`)
	}))
	defer server.Close()

	client := NewClient(Options{BaseURL: server.URL + "/api/v1/json/3"})
	events, err := client.EventsDayAll(context.Background(), "2099-01-01")
	if err != nil {
		t.Fatalf("EventsDayAll: %v", err)
	}
	if events == nil {
		t.Fatal("EventsDayAll returned nil slice; want empty non-nil")
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}
