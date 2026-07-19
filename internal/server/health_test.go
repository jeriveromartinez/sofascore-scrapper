package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
)

func TestHealthLiveReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	router.GET("/health/live", checker.LivenessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHealthLiveAlwaysReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	router.GET("/health/live", checker.LivenessHandler())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestHealthReadyReturns200WhenNoDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	router.GET("/health/ready", checker.ReadinessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with no registered deps, got %d", w.Code)
	}
}

func TestHealthReadyReturns200WhenDepsHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	checker.Register("database", func(ctx context.Context) error { return nil })
	checker.Register("redis", func(ctx context.Context) error { return nil })
	router.GET("/health/ready", checker.ReadinessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when deps healthy, got %d", w.Code)
	}
}

func TestHealthReadyReturns503WhenDepFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	checker.Register("database", func(ctx context.Context) error { return nil })
	checker.Register("redis", func(ctx context.Context) error { return fmt.Errorf("connection refused") })
	router.GET("/health/ready", checker.ReadinessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when dep fails, got %d", w.Code)
	}
}

func TestHealthReady503ContainsDependencyNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	checker.Register("database", func(ctx context.Context) error { return fmt.Errorf("timeout") })
	checker.Register("redis", func(ctx context.Context) error { return fmt.Errorf("dial error") })
	router.GET("/health/ready", checker.ReadinessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	for _, name := range []string{"database", "redis"} {
		if !contains(body, name) {
			t.Errorf("body should contain dep name '%s', got: %s", name, body)
		}
	}
	if contains(body, "timeout") || contains(body, "dial error") {
		t.Errorf("body should not contain error details, got: %s", body)
	}
}

func TestHealthReady503DoesNotLeakCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checker := NewHealthChecker()
	checker.Register("database", func(ctx context.Context) error {
		return fmt.Errorf("password=mypass host=db.example.com")
	})
	router.GET("/health/ready", checker.ReadinessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if contains(body, "mypass") {
		t.Errorf("body should not leak credentials: %s", body)
	}
	if contains(body, "password") {
		t.Errorf("body should not leak password string: %s", body)
	}
	if contains(body, "host") {
		t.Errorf("body should not leak host string: %s", body)
	}
}

func TestRequestIDFromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/test", func(c *gin.Context) {
		reqID, exists := c.Get("request_id")
		if !exists {
			t.Error("request_id should exist in gin context")
		}
		if reqID != "incoming-id-456" {
			t.Errorf("expected 'incoming-id-456', got '%v'", reqID)
		}

		logger, exists := c.Get("logger")
		if !exists || logger == nil {
			t.Error("logger should exist in gin context")
		}

		ctxLogger := observability.FromContext(c.Request.Context())
		if ctxLogger == nil {
			t.Error("logger should exist in request context")
		}

		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "incoming-id-456")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") != "incoming-id-456" {
		t.Errorf("response X-Request-ID: expected 'incoming-id-456', got '%s'", w.Header().Get("X-Request-ID"))
	}
}

func TestRequestIDGeneratedWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/test", func(c *gin.Context) {
		reqID, exists := c.Get("request_id")
		if !exists {
			t.Error("request_id should exist in gin context")
		}
		if reqID == "" {
			t.Error("request_id should not be empty")
		}
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	respID := w.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("response should contain X-Request-ID header")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
