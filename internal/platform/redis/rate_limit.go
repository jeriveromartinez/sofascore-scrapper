package redis

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const rateLimitKeyPrefix = "ratelimit"

var tokenBucketScript = goredis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_sec = math.ceil(tonumber(ARGV[2]) / 1000)

local current = redis.call("GET", key)
if current == false then
	redis.call("SET", key, 1, "EX", window_sec)
	return {1, tonumber(ARGV[2])}
end

local count = tonumber(current)
local ttl = redis.call("PTTL", key)
if ttl <= 0 then
	redis.call("SET", key, 1, "EX", window_sec)
	return {1, tonumber(ARGV[2])}
end

if count >= limit then
	return {0, ttl}
end

redis.call("INCR", key)
return {1, ttl}
`)

type RateLimitPolicy struct {
	Limit  int64
	Window time.Duration
}

type RateLimiter struct {
	client *goredis.Client
}

func NewRateLimiter(client *goredis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, policy RateLimitPolicy) (allowed bool, retryAfter time.Duration, err error) {
	if l.client == nil {
		return false, 0, fmt.Errorf("rate limit: redis client is nil")
	}

	hashedKey := rateLimitKeyPrefix + ":" + sha256Hash(key)
	windowMs := policy.Window.Milliseconds()

	result, err := tokenBucketScript.Run(ctx, l.client, []string{hashedKey}, policy.Limit, windowMs).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit check: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected rate limit script result: %v", result)
	}

	allowedVal, _ := values[0].(int64)
	retryMs, _ := values[1].(int64)

	if allowedVal == 1 {
		return true, 0, nil
	}

	return false, time.Duration(retryMs) * time.Millisecond, nil
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])
}

func Sha256Short(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:16])
}
