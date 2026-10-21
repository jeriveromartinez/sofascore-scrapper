# API Load Testing

k6 scenarios drive the wire format of every endpoint (protobuf in,
protobuf out). The scenarios live in `tests/load/`:

| File | Role |
|---|---|
| `common.js` | Shared helpers: `BASE_URL`, protobuf encoder / decoder, status checks, auth headers. **Only `BASE_URL` is read from the environment** (everything else is internal). |
| `pr-load.js` | PR-style load profile. The `load.yml` GitHub workflow runs this on `workflow_dispatch` with `suite=smoke`. **This is the scenario that fills the role of the missing `smoke.js`** — there is no separate smoke script. |
| `events.js` | Core API scenario (current-events, admin events, top-events) over 30 minutes with 50 VUs. |
| `devices.js` | Device registration + playback write load over 30 minutes with 40 VUs. |
| `playback.js` | Admin playback browse over 30 minutes with 20 VUs. |
| `uploads.js` | Chunked APK upload over 10 minutes with 3 VUs (relaxed thresholds). |
| `redis-degraded.js` | Induced Redis outage, asserts fail-closed behavior over 5 minutes with 10 VUs. |
| `multi-instance.js` | Staging / multi-backend scenario: scheduler lock sanity + DB-backed reads survive over 5 minutes with 15 VUs. |

## Thresholds

The values exported by `common.js`:

| Metric | Limit | Used for |
|---|---|---|
| `http_req_failed` | `rate < 0.01` (1%) | All scenarios that import `common.js`'s `thresholds`. |
| `http_req_duration{kind:read}` | `p(95) < 250ms` | Read-tagged GETs. |
| `http_req_duration{kind:write}` | `p(95) < 500ms` | Write-tagged POSTs / PUTs. |

| Scenario | Override |
|---|---|
| `uploads.js` | `http_req_failed: rate<0.05` (5%), `http_req_duration{kind:write}: p(95)<5000ms`. |
| `redis-degraded.js` | `http_req_failed: rate<0.10` (10%) to absorb the induced 503s; read p95 stays at 250ms. |
| `multi-instance.js` | `http_req_failed: rate<0.02`, `read p(95) < 300ms`, `write p(95) < 600ms` (slightly relaxed because three backends share the same load). |

## Running a scenario

```bash
k6 run tests/load/pr-load.js
k6 run tests/load/events.js
k6 run tests/load/devices.js
k6 run tests/load/playback.js
k6 run tests/load/uploads.js
k6 run tests/load/redis-degraded.js
k6 run tests/load/multi-instance.js
```

Every read in `common.js` resolves `BASE_URL` from the environment, so
override the target host per invocation:

```bash
BASE_URL=http://localhost:8080 k6 run tests/load/pr-load.js
BASE_URL=http://staging.example.com:8080 k6 run tests/load/events.js
```

`pr-load.js` additionally reads `INVITE_TOKEN` so the scenario can
register and promote its first admin against a fresh stack.

## Per-scenario notes

### `pr-load.js`

Ramps a controlled arrival rate against the admin read endpoints
(`/api/web/v1/devices`, `/api/web/v1/events`, `/api/web/v1/apk/versions`,
`/api/web/v1/stats/top-events`). Admin endpoints are rate-limited to
300 / minute per user, so the scenario seeds a small pool of admin
accounts (one bootstrap admin plus promoted invites) and spreads load
across them so the aggregate stays under the per-user budget. This is
the scenario the `load.yml` workflow runs with `suite=smoke`.

### `events.js`

`setup()` registers 20 devices; the default function hits current-events
as a device and admin events / top-events as admin, all read-tagged.
30 minutes, 50 VUs.

### `devices.js`

`setup()` registers 30 devices; the default function posts a playback
event and reads current-events. 30 minutes, 40 VUs.

### `playback.js`

Admin-only reading: `/api/web/v1/playback` list and `/api/web/v1/playback/page`
(cursor). 30 minutes, 20 VUs.

### `uploads.js`

Walks the full chunked-upload flow against
`/api/web/v1/apk/uploads`: `begin` (201), `PUT chunks/{i}` (200),
`complete` (200 or 409). Uses an in-memory 64 KiB chunk buffer. The
relaxed thresholds account for disk I/O on the chunk PUTs.

### `redis-degraded.js`

Confirms fail-closed behavior when Redis is unavailable:

- DB-backed event reads continue (`/api/app/v1/current-events`,
  `/api/web/v1/events`).
- `/health/ready` returns 503 while Redis is down.
- Device registration returns 503 because the invitation flow is
  Redis-backed (the scenario accepts 200 / 409 / 503 in its check).

The 10% error-rate threshold is **deliberately raised** to accommodate
the induced 503s.

### `multi-instance.js`

Used against `compose.multi.yml`. Asserts that scheduler-run counters
show exactly one execution across backends (the rest are skipped
because the Redis lock is held), that killing one backend does not
spike errors in the others, and that DB-backed reads keep working when
Redis is stopped.

## Environment variables

| Variable | Read by | Default | Description |
|---|---|---|---|
| `BASE_URL` | every scenario (via `common.js`) | `http://localhost:8080` | API server address. This is the **only** variable `common.js` consumes. |
| `INVITE_TOKEN` | `pr-load.js` only | *(empty)* | Bootstrap invitation token; required only when the fresh-stack branch registers the first admin. |

`TEST_EMAIL` and `TEST_PASSWORD` are not read from the environment by
this project — `common.js` does not export them. Each scenario calls
`loginUser(http, "admin@example.com", "admin123")` directly.

## Monitoring during a run

- **Prometheus** at `BASE_URL/metrics` — goroutines, DB pool, Redis
  timeouts.
- **Readiness** at `BASE_URL/health/ready` — DB + Redis status.
- **k6 output** — threshold violations, error rate, p95 latencies.
