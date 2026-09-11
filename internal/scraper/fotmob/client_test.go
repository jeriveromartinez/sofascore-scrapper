package fotmob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_ScheduledEvents_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"matches":[{"id":"4193492","home":{"id":1,"name":"H"},"away":{"id":2,"name":"A"},"status":{"code":1,"type":"scheduled"}}]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL})
	matches, err := c.ScheduledEvents(context.Background(), "47", "2026-09-10")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 1 || matches[0].Id != "4193492" {
		t.Fatalf("matches: %+v", matches)
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
		_, _ = w.Write([]byte(`{"matches":[]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 2, MaxBackoff: 2 * time.Second})
	_, err := c.ScheduledEvents(context.Background(), "47", "2026-09-10")
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
		_, _ = w.Write([]byte(`{"matches":[{"id":"ok","home":{"id":1,"name":"H"},"away":{"id":2,"name":"A"}}]}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		BaseURL:    server.URL,
		MaxRetries: 5,
		MaxBackoff: 2 * time.Second,
	})
	matches, err := c.ScheduledEvents(context.Background(), "47", "2026-09-10")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 retries + 1 success), got calls=%d", calls)
	}
	if len(matches) != 1 || matches[0].Id != "ok" {
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

	c := NewClient(ClientConfig{BaseURL: server.URL})
	_, err := c.ScheduledEvents(context.Background(), "47", "2026-09-10")
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

	c := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 5})
	_, err := c.ScheduledEvents(context.Background(), "47", "2026-09-10")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected single call (no retries on 4xx), got calls=%d", calls)
	}
}
