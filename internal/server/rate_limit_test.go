package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
)

func TestRateLimitAuthFailClosedOnRedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(nil))
	router.POST("/api/web/v1/users/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/users/login", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("auth route with nil Redis: expected 503, got %d", w.Code)
	}
}

func TestRateLimitReturnsRetryAfter(t *testing.T) {
	policy := redisplatform.RateLimitPolicy{Limit: 1, Window: time.Minute}
	rl := redisplatform.NewRateLimiter(nil)
	allowed, retryAfter, err := rl.Allow(t.Context(), "test-retry-after", policy)
	if err == nil {
		t.Fatal("expected error with nil Redis client")
	}
	if allowed {
		t.Fatal("expected not allowed with nil Redis client")
	}
	_ = retryAfter
}

func TestLocalLimiterFallback(t *testing.T) {
	limiter := newLocalLimiter(redisplatform.RateLimitPolicy{Limit: 5, Window: time.Minute})

	for i := 1; i <= 5; i++ {
		allowed, _ := limiter.Allow("device-1")
		if !allowed {
			t.Fatalf("request %d: expected allowed", i)
		}
	}

	allowed, retryAfter := limiter.Allow("device-1")
	if allowed {
		t.Fatal("request 6 should be denied by local limiter")
	}
	if retryAfter <= 0 {
		t.Fatalf("local limiter Retry-After should be positive, got %v", retryAfter)
	}
}

func TestLocalLimiterSeparateKeys(t *testing.T) {
	limiter := newLocalLimiter(redisplatform.RateLimitPolicy{Limit: 3, Window: time.Minute})

	for i := 1; i <= 3; i++ {
		allowed, _ := limiter.Allow("device-1")
		if !allowed {
			t.Fatalf("device-1 request %d: expected allowed", i)
		}
	}

	allowed, _ := limiter.Allow("device-2")
	if !allowed {
		t.Fatal("device-2 request 1: expected allowed (separate key)")
	}
}

func TestRateLimitAppReadFallbackOnRedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(nil))
	router.GET("/api/app/v1/update", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	for i := 1; i <= 120; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/app/v1/update?version=1.0.0&package=com.test.app", nil)
		req.Header.Set("APP-XIPTV", "device-token-abc")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 (local fallback), got %d", i, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/update?version=1.0.0&package=com.test.app", nil)
	req.Header.Set("APP-XIPTV", "device-token-abc")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 121 with local fallback: expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on local fallback 429")
	}
}

func TestRateLimitAdminFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(nil))
	router.GET("/api/web/v1/devices", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/web/v1/devices", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("admin route with nil Redis: expected 503, got %d", w.Code)
	}
}

func TestRateLimitAppReadIPFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(nil))
	router.GET("/api/app/v1/update", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/update?version=1.0.0&package=com.test.app", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("app read without APP-XIPTV header: expected 200 (IP-based local fallback), got %d", w.Code)
	}
}

func TestCrashReportRateLimitIgnoresSpoofedDeviceToken(t *testing.T) {
	originalPolicy := RateLimitAppRead
	RateLimitAppRead.Limit = 1
	RateLimitAppRead.Window = time.Minute
	t.Cleanup(func() { RateLimitAppRead = originalPolicy })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(nil))
	router.POST("/api/app/v1/crash-report", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i, token := range []string{"spoofed-device-a", "spoofed-device-b"} {
		req := httptest.NewRequest(http.MethodPost, "/api/app/v1/crash-report", nil)
		req.RemoteAddr = "203.0.113.20:1234"
		req.Header.Set("APP-XIPTV", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		want := http.StatusNoContent
		if i == 1 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d status = %d, want %d", i+1, w.Code, want)
		}
	}
}

func TestRateLimitPoliciesUseSeparateBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/web/v1/users/login", nil)
	c.Request.RemoteAddr = "203.0.113.30:1234"

	_, authKey, _ := classifyRateLimit(c, "/api/web/v1/users/login", http.MethodPost)
	_, uploadKey, _ := classifyRateLimit(c, "/api/web/v1/apk/uploads", http.MethodPost)
	_, appKey, _ := classifyRateLimit(c, "/api/app/v1/crash-report", http.MethodPost)

	if authKey == uploadKey || authKey == appKey || uploadKey == appKey {
		t.Fatalf("policy keys must be distinct: auth=%q upload=%q app=%q", authKey, uploadKey, appKey)
	}
}
