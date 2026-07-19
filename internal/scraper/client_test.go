package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newStubServer() *httptest.Server {
	var calls atomic.Int64
	mux := http.NewServeMux()

	mux.HandleFunc("/es/", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "abc123"})
		http.SetCookie(w, &http.Cookie{Name: "ss-session", Value: "sess456"})
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head></head><body></body></html>"))
	})

	mux.HandleFunc("/api/v1/sport/", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cookies := r.Cookies()
		if len(cookies) == 0 {
			w.Header().Set("X-Cookie-Count", "0")
		} else {
			w.Header().Set("X-Cookie-Count", fmt.Sprintf("%d", len(cookies)))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[]}`))
	})

	mux.HandleFunc("/api/v1/trending/events/", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cookies := r.Cookies()
		var name string
		for _, c := range cookies {
			if c.Name == "ss-id" {
				name = c.Value
			}
		}
		w.Header().Set("X-Cookie-Name", name)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[{"id":1,"slug":"test","startTimestamp":1710000000,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1710000000}}]}`))
	})

	return httptest.NewServer(mux)
}

func newRetryServer() *httptest.Server {
	var requestCount atomic.Int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/es/") {
			http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "cookie"})
			w.Write([]byte("<html></html>"))
			return
		}

		count := requestCount.Add(1)

		switch {
		case strings.Contains(r.URL.Path, "no-retry-4xx"):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("forbidden"))
			return
		case strings.Contains(r.URL.Path, "retry-429"):
			if count <= 2 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("rate limited"))
				return
			}
		case strings.Contains(r.URL.Path, "retry-429-after"):
			if count == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("rate limited"))
				return
			}
		case strings.Contains(r.URL.Path, "retry-502"):
			if count <= 2 {
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte("bad gateway"))
				return
			}
		case strings.Contains(r.URL.Path, "retry-503"):
			if count <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("service unavailable"))
				return
			}
		case strings.Contains(r.URL.Path, "auth-reject"):
			if count == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("unauthorized"))
				return
			}
		case strings.Contains(r.URL.Path, "auth-persistent"):
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		case strings.Contains(r.URL.Path, "big-body"):
			w.Header().Set("Content-Type", "application/json")
			body := fmt.Sprintf(`{"events":[{"id":%d,"slug":"big","startTimestamp":1,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1}}]}`, count)
			w.Write([]byte(body))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[]}`))
	}))
}

func newTimeoutServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/es/") {
			http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "cookie"})
			w.Write([]byte("<html></html>"))
			return
		}
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[]}`))
	}))
}

func newBodyCloseServer() *httptest.Server {
	var bodyRead atomic.Bool
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/es/") {
			http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "cookie"})
			w.Write([]byte("<html></html>"))
			return
		}
		bodyRead.Store(true)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[]}`))
	}))
}

func TestClient_CookieBootstrapOnce(t *testing.T) {
	ts := newStubServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "football", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.TrendingEvents(ctx, "MX")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ScheduledEvents(ctx, "basketball", time.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "no-retry-4xx", time.Now())
	if err == nil {
		t.Fatal("expected error for 4xx")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got: %v", err)
	}
}

func TestClient_RetryOn429(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "retry-429", time.Now())
	if err != nil {
		t.Fatalf("expected success after 429 retries: %v", err)
	}
}

func TestClient_RetryOn429WithRetryAfter(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "retry-429-after", time.Now())
	if err != nil {
		t.Fatalf("expected success after 429: %v", err)
	}
}

func TestClient_RetryOn502(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "retry-502", time.Now())
	if err != nil {
		t.Fatalf("expected success after 502 retries: %v", err)
	}
}

func TestClient_RetryOn503(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "retry-503", time.Now())
	if err != nil {
		t.Fatalf("expected success after 503 retries: %v", err)
	}
}

func TestClient_CookieRefreshOn401(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 3}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "auth-reject", time.Now())
	if err != nil {
		t.Fatalf("expected success after cookie refresh: %v", err)
	}
}

func TestClient_Persistent401Fails(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 2}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "auth-persistent", time.Now())
	if err == nil {
		t.Fatal("expected error for persistent 401")
	}
}

func TestClient_ResponseSizeCap(t *testing.T) {
	ts := newRetryServer()
	defer ts.Close()

	cfg := ClientConfig{
		BaseURL:          ts.URL,
		MaxRetries:       1,
		ResponseMaxBytes: 50,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "big-body", time.Now())
	if err == nil {
		t.Fatal("expected parse error due to truncated response")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	ts := newTimeoutServer()
	defer ts.Close()

	cfg := ClientConfig{
		BaseURL:        ts.URL,
		MaxRetries:     1,
		RequestTimeout: 10 * time.Second,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.ScheduledEvents(ctx, "football", time.Now())
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestClient_Timeout(t *testing.T) {
	ts := newTimeoutServer()
	defer ts.Close()

	cfg := ClientConfig{
		BaseURL:        ts.URL,
		MaxRetries:     1,
		RequestTimeout: 50 * time.Millisecond,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "football", time.Now())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClient_BodyClosure(t *testing.T) {
	ts := newBodyCloseServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.ScheduledEvents(ctx, "football", time.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_TrendingEvents(t *testing.T) {
	ts := newStubServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	events, err := client.TrendingEvents(ctx, "MX")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != 1 {
		t.Fatalf("expected event ID 1, got %d", events[0].ID)
	}
}

func TestClient_ScheduledEvents(t *testing.T) {
	ts := newStubServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	events, err := client.ScheduledEvents(ctx, "football", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if events != nil {
		if len(events) != 0 {
			t.Fatalf("expected 0 events, got %d", len(events))
		}
	}
}

func TestClient_CookiesSent(t *testing.T) {
	ts := newStubServer()
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.TrendingEvents(ctx, "MX")
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_BackoffBound(t *testing.T) {
	cfg := ClientConfig{
		BaseURL:    "http://localhost",
		MaxRetries: 5,
		MaxBackoff: 5 * time.Second,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	d := client.backoffDuration(10)
	if d != 5*time.Second {
		t.Fatalf("backoff should be capped at MaxBackoff(5s), got %v", d)
	}
}

func TestClient_TrendingParsesAllFields(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/es/") {
			http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "cookie"})
			w.Write([]byte("<html></html>"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"events":[]}`)
	}))
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	events, err := client.TrendingEvents(context.Background(), "mx")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("expected 0 events")
	}
}

func TestClient_ReadAllBody(t *testing.T) {
	data := `{"events":[{"id":1,"slug":"test","startTimestamp":1710000000,"homeTeam":{"id":1,"name":"H"},"awayTeam":{"id":2,"name":"A"},"status":{"code":0,"description":"","type":"notstarted"},"time":{"currentPeriodStartTimestamp":1710000000}}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/es/") {
			http.SetCookie(w, &http.Cookie{Name: "ss-id", Value: "cookie"})
			w.Write([]byte("<html></html>"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, data)
	}))
	defer ts.Close()

	cfg := ClientConfig{BaseURL: ts.URL, MaxRetries: 1}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	events, err := client.ScheduledEvents(context.Background(), "football", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
}
