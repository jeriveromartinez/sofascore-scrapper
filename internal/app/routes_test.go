package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"google.golang.org/protobuf/proto"
)

func TestRouterIgnoresForwardedForByDefault(t *testing.T) {
	router := newClientIPTestRouter(t, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"})
	req := httptest.NewRequest(http.MethodGet, "/test/client-ip", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.25")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "203.0.113.10" {
		t.Fatalf("client IP=%q, want direct peer", w.Body.String())
	}
}

func TestRouterUsesForwardedForOnlyFromTrustedProxy(t *testing.T) {
	cfg := config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}
	cfg.HTTP.TrustedProxies = []string{"203.0.113.10"}
	router := newClientIPTestRouter(t, cfg)

	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "trusted", remoteAddr: "203.0.113.10:1234", want: "198.51.100.25"},
		{name: "untrusted", remoteAddr: "203.0.113.11:1234", want: "203.0.113.11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test/client-ip", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("X-Forwarded-For", "198.51.100.25")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
			}
			if w.Body.String() != tt.want {
				t.Fatalf("client IP=%q, want %q", w.Body.String(), tt.want)
			}
		})
	}
}

func newClientIPTestRouter(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()
	router := newTestRouter(t, cfg)
	router.GET("/test/client-ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	return router
}

func newTestRouter(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()
	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(nil, nil, cfg, tokens, nil, realtime.NewHub(), nil)
}

func TestCrashReportInheritsOneMiBBodyLimit(t *testing.T) {
	router := newTestRouter(t, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/app/v1/crash-report",
		bytes.NewReader(make([]byte, (1<<20)+1)),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCrashReportInheritsAppIPRateLimit(t *testing.T) {
	originalPolicy := server.RateLimitAppRead
	server.RateLimitAppRead.Limit = 1
	server.RateLimitAppRead.Window = time.Minute
	t.Cleanup(func() { server.RateLimitAppRead = originalPolicy })

	router := newTestRouter(t, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/app/v1/crash-report", strings.NewReader("{"))
		req.RemoteAddr = "203.0.113.20:1234"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := request(); w.Code != http.StatusBadRequest {
		t.Fatalf("first status=%d, want %d", w.Code, http.StatusBadRequest)
	}
	if w := request(); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status=%d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestAdminRateLimitRunsBeforeProtectedHandler(t *testing.T) {
	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	accessToken, err := tokens.GenerateAccessToken(42, "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(&pb.UploadBeginRequest{
		FileName:    "test.apk",
		FileSize:    1024,
		TotalChunks: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(nil, nil, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}, tokens, nil, realtime.NewHub(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/apk/uploads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want fail-closed %d before handler execution; body=%q", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

func TestRouteCompatibility(t *testing.T) {
	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}
	router := NewRouter(nil, nil, cfg, tokens, nil, realtime.NewHub(), nil)
	got := make(map[string]bool)
	for _, route := range router.Routes() {
		got[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /health/live",
		"GET /health/ready",
		"GET /metrics",
		"GET /api/app/v1/update",
		"GET /api/app/v1/apk/download/:token",
		"GET /api/app/v1/current-events",
		"POST /api/app/v1/devices",
		"POST /api/app/v1/devices/viewing",
		"GET /api/app/v1/devices/url/:packageName",
		"POST /api/app/v1/crash-report",
		"GET /api/app/v1/teams/logo/:teamId",
		"POST /api/web/v1/users/register",
		"POST /api/web/v1/users/login",
		"POST /api/web/v1/users/refresh",
		"POST /api/web/v1/users/logout",
		"POST /api/web/v1/users/invitations",
		"GET /api/web/v1/users",
		"GET /api/web/v1/users/page",
		"GET /api/web/v1/users/:id",
		"POST /api/web/v1/users",
		"PUT /api/web/v1/users/:id",
		"PUT /api/web/v1/users/:id/role",
		"DELETE /api/web/v1/users/:id",
		"GET /api/web/v1/events",
		"GET /api/web/v1/events/page",
		"GET /api/web/v1/devices",
		"GET /api/web/v1/devices/page",
		"GET /api/web/v1/devices/all",
		"PUT /api/web/v1/devices",
		"GET /api/web/v1/playback",
		"GET /api/web/v1/playback/page",
		"GET /api/web/v1/stats/top-events",
		"POST /api/web/v1/apk/upload",
		"POST /api/web/v1/apk/upload/chunk",
		"POST /api/web/v1/apk/upload/assemble",
		"GET /api/web/v1/apk/versions",
		"GET /api/web/v1/apk/versions/page",
		"PUT /api/web/v1/apk/:id",
		"POST /api/web/v1/apk/uploads",
		"GET /api/web/v1/apk/uploads/:id",
		"PUT /api/web/v1/apk/uploads/:id/chunks/:index",
		"POST /api/web/v1/apk/uploads/:id/complete",
		"DELETE /api/web/v1/apk/uploads/:id",
		"GET /api/web/v1/tournaments",
		"GET /api/web/v1/tournaments/page",
		"GET /api/web/v1/tournaments/:id",
		"POST /api/web/v1/tournaments",
		"PUT /api/web/v1/tournaments/:id",
		"DELETE /api/web/v1/tournaments/:id",
		"GET /api/web/v1/device-tournaments",
		"GET /api/web/v1/device-tournaments/page",
		"GET /api/web/v1/device-tournaments/:deviceId",
		"POST /api/web/v1/device-tournaments",
		"DELETE /api/web/v1/device-tournaments",
		"PUT /api/web/v1/device-tournaments/:deviceId",
		"GET /api/web/v1/global-tournament-config",
		"POST /api/web/v1/global-tournament-config",
		"DELETE /api/web/v1/global-tournament-config/:tournamentId",
		"GET /api/web/v1/domains",
		"GET /api/web/v1/domains/page",
		"GET /api/web/v1/domains/:id",
		"POST /api/web/v1/domains",
		"PUT /api/web/v1/domains/:id",
		"DELETE /api/web/v1/domains/:id",
	} {
		if !got[want] {
			t.Errorf("missing route %s", want)
		}
	}
	if len(got) != 66 {
		t.Fatalf("got %d routes, want 66 (the +1 is /api/app/v1/ws for the push realtime endpoint; the other new ones are the /api/admin/v1/pushes surface)", len(got))
	}
}
