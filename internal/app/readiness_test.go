package app

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/auth"
	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
	"github.com/jeriveromartinez/scores365-scrapper/internal/realtime"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newReadinessTestRouter(t *testing.T, ready *atomic.Bool) http.Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	cfg := config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}
	return NewRouter(db, redisClient, cfg, tokens, nil, realtime.NewHub(), nil, ready)
}

// TestReadinessGateRejectsAPIWhenNotReady asserts the API groups
// answer 503 while the app is not serving, while the liveness probe
// stays reachable so the orchestrator can distinguish "starting" from
// "crashed", and the readiness probe fails so the load balancer stops
// routing traffic to a draining instance.
func TestReadinessGateRejectsAPIWhenNotReady(t *testing.T) {
	var ready atomic.Bool
	router := newReadinessTestRouter(t, &ready)

	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/current-events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("api status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	// The admin groups inherit the gate as well, and it must fire before
	// authentication: an unauthenticated call answers 503, not 401.
	req = httptest.NewRequest(http.MethodGet, "/api/web/v1/scraper-leagues", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("admin api status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	req = httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("liveness status=%d, want %d", w.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestReadinessGateAllowsAPIWhenReady asserts the gate opens once the
// app finished booting.
func TestReadinessGateAllowsAPIWhenReady(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)
	router := newReadinessTestRouter(t, &ready)

	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/current-events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == http.StatusServiceUnavailable {
		t.Fatal("status=503, want the route to be served when the app is ready")
	}

	req = httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("readiness status=%d, want %d", w.Code, http.StatusOK)
	}
}
