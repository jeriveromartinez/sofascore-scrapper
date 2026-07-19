package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	goredis "github.com/redis/go-redis/v9"
)

const localFallbackMaxKeys = 10_000

var (
	RateLimitAuth      = redisplatform.RateLimitPolicy{Limit: 10, Window: time.Minute}
	RateLimitAdmin     = redisplatform.RateLimitPolicy{Limit: 300, Window: time.Minute}
	RateLimitDeviceReg = redisplatform.RateLimitPolicy{Limit: 20, Window: time.Minute}
	RateLimitAppRead   = redisplatform.RateLimitPolicy{Limit: 120, Window: time.Minute}
	RateLimitPlayback  = redisplatform.RateLimitPolicy{Limit: 30, Window: time.Minute}
)

type localBucket struct {
	tokens     float64
	lastRefill time.Time
}

type localLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*localBucket
	capacity float64
	rate     float64
	window   time.Duration
	now      func() time.Time
}

func newLocalLimiter(policy redisplatform.RateLimitPolicy) *localLimiter {
	return &localLimiter{
		buckets:  make(map[string]*localBucket),
		capacity: float64(policy.Limit),
		rate:     float64(policy.Limit) / policy.Window.Seconds(),
		window:   policy.Window,
		now:      time.Now,
	}
}

func (l *localLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.evictExpired()

	b, exists := l.buckets[key]
	now := l.now()

	if !exists {
		if len(l.buckets) >= localFallbackMaxKeys {
			return false, l.window
		}
		b = &localBucket{tokens: l.capacity, lastRefill: now}
		l.buckets[key] = b
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.lastRefill = now

	if b.tokens < 1 {
		refillTime := (1 - b.tokens) / l.rate
		return false, time.Duration(refillTime * float64(time.Second))
	}

	b.tokens--
	return true, 0
}

func (l *localLimiter) evictExpired() {
	now := l.now()
	for k, b := range l.buckets {
		if now.Sub(b.lastRefill) > l.window && b.tokens >= l.capacity {
			delete(l.buckets, k)
		}
	}
}

func RateLimit(redisClient *goredis.Client) gin.HandlerFunc {
	redisLimiter := redisplatform.NewRateLimiter(redisClient)
	localAppRead := newLocalLimiter(RateLimitAppRead)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		policy, key, failClosed := classifyRateLimit(c, path, method)

		allowed, retryAfter, err := redisLimiter.Allow(c.Request.Context(), key, policy)
		if err != nil {
			if failClosed {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "rate limit service unavailable"})
				return
			}
			allowed, retryAfter = localAppRead.Allow(key)
		}

		if !allowed {
			retrySeconds := int64(retryAfter.Seconds())
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			c.Header("Retry-After", strconv.FormatInt(retrySeconds, 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}

func classifyRateLimit(c *gin.Context, path string, method string) (redisplatform.RateLimitPolicy, string, bool) {
	if strings.HasPrefix(path, "/api/web/v1/apk/upload") {
		return RateLimitAdmin, c.ClientIP(), true
	}

	if strings.HasPrefix(path, "/api/web/v1/users/register") ||
		strings.HasPrefix(path, "/api/web/v1/users/login") ||
		strings.HasPrefix(path, "/api/web/v1/users/refresh") {
		return RateLimitAuth, c.ClientIP(), true
	}

	if strings.HasPrefix(path, "/api/web/v1/") {
		return RateLimitAdmin, resolveUserOrIP(c), true
	}

	if strings.HasPrefix(path, "/api/app/v1/devices/viewing") && method == http.MethodPost {
		deviceKey := resolveDeviceOrIP(c)
		return RateLimitPlayback, deviceKey, false
	}

	if strings.HasPrefix(path, "/api/app/v1/devices") && method == http.MethodPost {
		return RateLimitDeviceReg, c.ClientIP(), true
	}

	if strings.HasPrefix(path, "/api/app/v1/") {
		deviceKey := resolveDeviceOrIP(c)
		return RateLimitAppRead, deviceKey, false
	}

	return RateLimitAppRead, c.ClientIP(), false
}

func resolveUserOrIP(c *gin.Context) string {
	if userID, exists := c.Get("userID"); exists {
		return fmt.Sprintf("user:%d", userID)
	}
	return c.ClientIP()
}

func resolveDeviceOrIP(c *gin.Context) string {
	if token := c.GetHeader("APP-XIPTV"); token != "" {
		return "device:" + redisplatform.Sha256Short(token)
	}
	return c.ClientIP()
}


