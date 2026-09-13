package fotmob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestClient_ScheduledEvents_OK verifies the happy path against the
// real /api/data/matches endpoint shape (PR #124). The fake server
// only returns 200 if the request URL matches the documented path,
// so a regression to the legacy /api/leagues endpoint path would
// surface here.
func TestClient_ScheduledEvents_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PR #124: real endpoint is /api/data/matches, NOT /api/leagues.
		if !strings.HasPrefix(r.URL.Path, "/api/data/matches") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Date must be in YYYYMMDD with no hyphens.
		if got := r.URL.Query().Get("date"); got != "20260911" {
			t.Errorf("date param: want 20260911, got %q", got)
		}
		if got := r.URL.Query().Get("timezone"); got != "Europe/Paris" {
			t.Errorf("timezone param: want Europe/Paris, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"leagues": [
				{
					"id": 54,
					"name": "Bundesliga",
					"matches": [
						{
							"id": 5881169,
							"leagueId": 54,
							"home": {"id": 1, "score": 2, "name": "Home"},
							"away": {"id": 2, "score": 1, "name": "Away"},
							"statusId": 6,
							"status": {"finished": true, "started": true, "scoreStr": "2 - 1"}
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	matches, err := c.ScheduledEvents(context.Background(), "54", date)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches len: want 1, got %d", len(matches))
	}
	if matches[0].Id != 5881169 {
		t.Errorf("match Id: want 5881169, got %d", matches[0].Id)
	}
	if matches[0].Home.Score != 2 {
		t.Errorf("home score: want 2, got %d", matches[0].Home.Score)
	}
}

// TestClient_ScheduledEvents_PerLeagueFilter verifies that the client
// fetches the full day's payload and returns ONLY the matches for
// the requested league (PR #124). The /api/data/matches endpoint
// has no per-league filter; the client must filter client-side.
func TestClient_ScheduledEvents_PerLeagueFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return two leagues with matches; the client must keep
		// only Bundesliga and drop LaLiga.
		_, _ = w.Write([]byte(`{
			"leagues": [
				{
					"id": 54,
					"name": "Bundesliga",
					"matches": [
						{"id": 1, "leagueId": 54, "home": {"id": 10, "score": 0, "name": "BH"}, "away": {"id": 11, "score": 0, "name": "BA"}, "statusId": 0, "status": {"finished": false, "started": false, "scoreStr": ""}}
					]
				},
				{
					"id": 87,
					"name": "LaLiga",
					"matches": [
						{"id": 2, "leagueId": 87, "home": {"id": 20, "score": 0, "name": "LH"}, "away": {"id": 21, "score": 0, "name": "LA"}, "statusId": 0, "status": {"finished": false, "started": false, "scoreStr": ""}}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	matches, err := c.ScheduledEvents(context.Background(), "54", date)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches len: want 1 (Bundesliga only), got %d", len(matches))
	}
	if matches[0].LeagueId != 54 {
		t.Errorf("match LeagueId: want 54, got %d", matches[0].LeagueId)
	}
}

// TestClient_ScheduledEvents_LeagueNotFound returns an empty slice
// (no error) when the requested league is not present in the day's
// payload. The scraper treats an empty day as a valid no-op.
func TestClient_ScheduledEvents_LeagueNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"leagues":[{"id":54,"name":"Bundesliga","matches":[]}]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	matches, err := c.ScheduledEvents(context.Background(), "999", date)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches len: want 0, got %d", len(matches))
	}
}

func TestClient_ScheduledEvents_429_Retries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"leagues":[]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 2, MaxBackoff: 2 * time.Second, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	_, err := c.ScheduledEvents(context.Background(), "54", date)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected retry, calls=%d", calls)
	}
}

// TestClient_ScheduledEvents_500_Retries verifies exponential backoff retry on 5xx.
func TestClient_ScheduledEvents_500_Retries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"leagues":[{"id":54,"name":"B","matches":[{"id":99,"leagueId":54,"home":{"id":1,"score":0,"name":"H"},"away":{"id":2,"score":0,"name":"A"},"statusId":0,"status":{"finished":false,"started":false,"scoreStr":""}}]}]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 5,
		MaxBackoff: 2 * time.Second,
		Timezone:   "Europe/Paris",
	})
	date, _ := time.Parse("20060102", "20260911")
	matches, err := c.ScheduledEvents(context.Background(), "54", date)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 retries + 1 success), got calls=%d", calls)
	}
	if len(matches) != 1 || matches[0].Id != 99 {
		t.Fatalf("matches: %+v", matches)
	}
}

// TestClient_ScheduledEvents_MalformedJSON ensures the client returns a parse error
// (not a silent zero result) when the server returns invalid JSON.
func TestClient_ScheduledEvents_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not-json`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	_, err := c.ScheduledEvents(context.Background(), "54", date)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

// TestClient_ScheduledEvents_4xx_NoRetry ensures 4xx (non-429) is terminal and not retried.
func TestClient_ScheduledEvents_4xx_NoRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`forbidden`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 5, Timezone: "Europe/Paris"})
	date, _ := time.Parse("20060102", "20260911")
	_, err := c.ScheduledEvents(context.Background(), "54", date)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected single call (no retries on 4xx), got calls=%d", calls)
	}
}

// TestClient_DayMatches_DateUsesConfiguredTimezone is the regression
// test for the P1 Codex finding on PR #124: the per-day query
// parameter must reflect the configured timezone (default
// Europe/Paris), not the host's local time.
//
// Scenario: a Docker container whose host clock is UTC runs the
// scraper at 22:30 UTC = 00:30 Europe/Paris on the NEXT calendar
// day. The "current day" in Paris is therefore the next day, but
// date.UTC().Format(...) returns today's UTC date — so the query
// fetches yesterday's matches in Paris.
//
// The fix: compute the date in c.timezone before formatting. This
// test captures the URL the client actually built and asserts that
// the date field reflects the configured timezone's local day.
func TestClient_DayMatches_DateUsesConfiguredTimezone(t *testing.T) {
	cases := []struct {
		name          string
		timezone      string
		// "now" expressed as a UTC instant. The cron would call
		// dayMatches with date=time.Now() — for the test we pass an
		// explicit instant so the assertion is deterministic.
		now           time.Time
		wantDateParam string
	}{
		{
			// 22:30 UTC = 00:30 CEST on Sept 15. With timezone=Paris
			// the request must ask for Sept 15, not Sept 14.
			name:          "paris_after_midnight",
			timezone:      "Europe/Paris",
			now:           time.Date(2026, 9, 14, 22, 30, 0, 0, time.UTC),
			wantDateParam: "20260915",
		},
		{
			// 21:30 UTC = 16:30 CDT on Sept 14. With timezone=Chicago
			// the request must ask for Sept 14.
			name:          "chicago_afternoon",
			timezone:      "America/Chicago",
			now:           time.Date(2026, 9, 14, 21, 30, 0, 0, time.UTC),
			wantDateParam: "20260914",
		},
		{
			// 06:00 UTC on Sept 15 — both UTC and Europe/Paris agree.
			name:          "paris_morning_no_boundary",
			timezone:      "Europe/Paris",
			now:           time.Date(2026, 9, 15, 6, 0, 0, 0, time.UTC),
			wantDateParam: "20260915",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.String()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"leagues":[]}`))
			}))
			defer server.Close()

			c := NewClient(ClientConfig{
				BaseURL:    server.URL,
				Timezone:   tc.timezone,
				MaxRetries: 1,
			})
			if _, err := c.dayMatches(context.Background(), tc.now); err != nil {
				t.Fatalf("dayMatches: %v", err)
			}
			wantSubstr := "date=" + tc.wantDateParam
			if !strings.Contains(capturedPath, wantSubstr) {
				t.Errorf("path = %q, want it to contain %q (configured TZ %s, host UTC %s)",
					capturedPath, wantSubstr, tc.timezone, tc.now.Format(time.RFC3339))
			}
			// The configured timezone must also appear in the URL so
			// FotMob buckets correctly.
			wantTzSubstr := "timezone=" + tc.timezone
			if !strings.Contains(capturedPath, wantTzSubstr) {
				t.Errorf("path = %q, want it to contain %q", capturedPath, wantTzSubstr)
			}
		})
	}
}
