# API Load Testing

## Overview

k6 load tests enforce API performance objectives using realistic traffic patterns.
Tests use protobuf-encoded request bodies matching the application's `application/x-protobuf` content type.

## Thresholds

| Metric | Limit | Scope |
|--------|-------|-------|
| `http_req_failed` | `< 1%` | error rate across all requests |
| `http_req_duration{kind:read}` | `p95 < 250ms` | GET / HEAD requests tagged `{kind:read}` |
| `http_req_duration{kind:write}` | `p95 < 500ms` | POST / PUT requests tagged `{kind:write}` |

Upload tests use relaxed thresholds (`p95 < 5s` for writes) since chunked uploads include disk I/O.

## Scenarios

### Smoke (`smoke.js`)
- **Purpose**: PR gate, runs on every push
- **Duration**: 2 minutes
- **VUs**: 5
- **Endpoints**: health/live, health/ready, admin devices, admin events, apk versions, stats/top-events
- **Threshold**: `p95 < 800ms` overall (relaxed for short run)

### Events (`events.js`)
- **Purpose**: Core API scenario
- **Duration**: 30 minutes
- **VUs**: 50
- **Endpoints**: current-events (app), events (admin), events/page (admin), stats/top-events
- **Setup**: Pre-registers 20 devices

### Devices (`devices.js`)
- **Purpose**: Device registration and playback write load
- **Duration**: 30 minutes
- **VUs**: 40
- **Endpoints**: POST devices/viewing (write), GET current-events (read)
- **Setup**: Pre-registers 30 devices

### Playback (`playback.js`)
- **Purpose**: Admin playback report browsing
- **Duration**: 30 minutes
- **VUs**: 20
- **Endpoints**: GET playback (list), GET playback/page (cursor)

### Uploads (`uploads.js`)
- **Purpose**: Chunked APK upload flow
- **Duration**: 10 minutes
- **VUs**: 3
- **Endpoints**: POST uploads/begin, PUT chunks, POST uploads/complete
- **Relaxed thresholds**: `p95 < 5s` for writes, `< 5%` error rate

### Redis Degraded (`redis-degraded.js`)
- **Purpose**: Validate fail-closed behavior when Redis is unavailable
- **Duration**: 5 minutes
- **VUs**: 10
- **Expectations**:
  - DB-backed event reads continue (`/api/app/v1/current-events`, `/api/web/v1/events`)
  - Health ready returns 503 when Redis is down
  - Device registration may return 503 (Redis-backed invitation flow)
  - Error rate threshold raised to 10% to accommodate induced 503s

### Multi-Instance (`multi-instance.js`)
- **Purpose**: Verify scheduler runs once across instances via Redis lock
- **Duration**: 5 minutes
- **VUs**: 15
- **Endpoints**: health, current-events, admin events, devices, stats, metrics
- **Validation**:
  - Scheduler run counters show exactly one execution across all instances
  - Kill one backend instance: no error spike in remaining instances
  - Stop Redis: DB-backed reads continue; fail-closed flows return 503

## Running Tests

```bash
k6 run tests/load/smoke.js
k6 run --duration 30m tests/load/events.js
k6 run tests/load/devices.js
k6 run tests/load/playback.js
k6 run tests/load/uploads.js
k6 run tests/load/redis-degraded.js
k6 run tests/load/multi-instance.js
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:8080` | API server address |
| `TEST_EMAIL` | `admin@example.com` | Admin user email |
| `TEST_PASSWORD` | `admin123` | Admin user password |

### Full Profile

```bash
k6 run --duration 30m tests/load/events.js &
k6 run --duration 30m tests/load/devices.js &
k6 run --duration 30m tests/load/playback.js &
wait
```

## Monitoring

During load tests, monitor:

- **Prometheus** at `/metrics`: goroutine count, SQL connection pool, Redis timeout growth
- **Health** at `/health/ready`: dependency status
- **k6 output**: threshold violations, error rate, p95 latencies
