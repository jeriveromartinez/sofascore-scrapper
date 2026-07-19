### Task 4 Report: Limitar servidor, cuerpos y frecuencia

**Status:** Complete

---

#### Files created
- `internal/platform/redis/rate_limit.go` — Lua-based token bucket `RateLimiter.Allow` with SHA-256 key hashing
- `internal/platform/redis/rate_limit_integration_test.go` — Integration tests (10/min, shared state, nil-client error)
- `internal/server/body_limit.go` — `BodyLimit()` middleware with `http.MaxBytesReader`
- `internal/server/body_limit_test.go` — 1 MiB accepted, 1 MiB+1 rejected (413), upload/chunk routes allow larger payloads
- `internal/server/rate_limit.go` — `RateLimit()` middleware with policy classification, fail-closed for auth/admin, local token bucket fallback for app reads
- `internal/server/rate_limit_test.go` — Tests: auth fail-closed (503), admin fail-closed (503), app-read local fallback (120 OK, 121→429+Retry-After), IP-based fallback

#### Files modified
- `internal/app/app.go` — Explicit `*http.Server` with `ReadHeaderTimeout: 5s`, `ReadTimeout: 15m`, `WriteTimeout: 60s`, `IdleTimeout: 60s`, `MaxHeaderBytes: 1<<20`
- `internal/app/routes.go` — Middleware order: `gin.Recovery() → requestID() → BodyLimit() → CORS() → Logger() → RateLimit()`, added request ID middleware

#### Architecture decisions

1. **Rate limit is global middleware** — In Gin, per-route auth runs after global middleware. Since userID isn't available yet at rate-limit time, all routes are keyed by IP or device header. This still provides effective rate limiting.

2. **Fail-closed vs fail-open:** Auth routes (login, register, refresh) and admin routes return 503 when Redis is unavailable. App-read routes use a bounded in-memory token bucket (10,000 key cap with expiry eviction).

3. **Body limits:** Protobuf routes capped at 1 MiB, direct upload at 200 MiB+1 MiB, chunk upload at 10 MiB+1 MiB. Uses `Content-Length` early rejection + `http.MaxBytesReader` wrapping.

4. **Lua script:** Atomic INCR + first-use SET with TTL, returns `{allowed, retryAfterMs}`. Keys are SHA-256 hashed, rate-limit-key prefixed.

#### Test results
```
ok  github.com/jeriveromartinez/sofascore-scrapper/internal/server      0.696s
ok  github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis  0.653s
ok  github.com/jeriveromartinez/sofascore-scrapper/internal/app          0.116s
go vet: clean
```
