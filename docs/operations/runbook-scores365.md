# Runbook — 365scores scraper source

**Audience:** on-call operator. **Assumes:** basic Go + docker + SQL knowledge.

This runbook describes how the 365scores source behaves, how discovery and
the cron tick actually run, the limits of the upstream feed, and the two
rollback paths available when 365scores is causing production issues.

## Prerequisites

The behavior described here requires the catalog discovery path
(`internal/scraper/catalog/discovery.go`), the source wiring
(`internal/scraper/scores365/source.go`), and the scheduler entrypoints
in `internal/scheduler/scrape.go` to all be present in the deployed build.
These landed in PR #134 (sitemap-based discovery — superseded, see
"Discovery" below), PR #135 (source wiring + dispatch), and PR #136 (this
runbook). Verify with:

```bash
git log --oneline | grep -E '#13[456]'
```

If the relevant commits are not reachable from the deployed branch, the
features described here are not yet live and this runbook does not apply.

## Configuration

Every 365scores knob is optional; the defaults match
<https://www.365scores.com/es-mx> and need no `/etc/iptv/env` entry.
See [`README.md`](README.md#environment-variables) for the full table.

| Variable | Default | What it changes |
|---|---|---|
| `SCORES365_LANG_ID` | `29` (es-mx) | Language of league, team, and status text in the feed. The cancelled/postponed keyword table matches Spanish and English only; changing this to a third language causes those statuses to fall through to `finished`. |
| `SCORES365_TIMEZONE` | `UTC` | IANA zone the feed uses to bucket one day of fixtures. |
| `SCORES365_COUNTRY_ID` | `116` | Popularity ordering of the competitions catalog only. It does not filter which competitions come back. |
| `SCORES365_DISCOVERY_SPORTS` | built-in list | Comma-separated sport slugs the discovery sweep enrolls. Empty means every sport except football (FotMob owns football). Unknown slugs are logged and skipped, so a typo silently narrows the sweep — check the `catalog: discovery unknown sport slug` warning after changing it. |

## Post-deploy verification (smoke test)

`DB_PASSWORD` must be exported (see `.env.example` for the default in dev).

Run after `git pull` on the backend host, BEFORE promoting the change to
production:

1. Full reset:
   ```bash
   docker compose -f deployments/docker/compose.dev.yml down -v
   docker compose -f deployments/docker/compose.dev.yml up -d --build backend
   ```
2. Wait ~60s for boot, first discovery sweep, and the first cron tick.
3. Verify the catalog:
   ```bash
   docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" scores365 -e \
     "SELECT source, sport, COUNT(*) FROM scraper_leagues GROUP BY source, sport ORDER BY source, sport;"
   ```
   Expect rows for both `fotmob` and `scores365`. `scores365` covers
   basketball, tennis, ice-hockey, handball, american-football, baseball,
   volleyball, rugby (football is intentionally not enrolled for 365scores).
4. Verify events landed:
   ```bash
   docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" scores365 -e \
     "SELECT source, sport, COUNT(*) FROM events GROUP BY source, sport ORDER BY source, sport;"
   ```
5. Hit the public events endpoint. `compose.dev.yml` maps host `8080` to
   container `8080` (`"${API_PORT:-8080}:8080"` on line 45):
   ```bash
   curl -i 'http://localhost:8080/api/app/v1/current-events?limit=6'
   ```
   Expect HTTP 200 with team names populated. `limit` is capped at 6
   (`currentEventsMaxLimit` in `internal/events/app_handler.go`); a
   larger value is answered with `400 limit must be between 1 and 6`.
6. Verify a team logo URL serves correctly:
   ```bash
   TEAM_ID=$(docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" scores365 -Nse "SELECT team_id FROM teams ORDER BY team_id DESC LIMIT 1;")
   curl -i "http://localhost:8080/api/app/v1/teams/logo/$TEAM_ID" -o /tmp/logo.png
   file /tmp/logo.png   # expect: PNG image data
   ```

## What 365scores gives us

The 365scores source is a thin adapter over
`https://webws.365scores.com`, calling two endpoints:

- **`/web/games/allscores/`** — every fixture for a given calendar day,
  across every sport. The day is passed explicitly as a
  `startDate`/`endDate` pair and is bucketed in `SCORES365_TIMEZONE`.
- **`/web/competitions/`** — the full competition catalog, with a
  `?sports=id1,id2,...` filter. Discovery uses this to enroll leagues
  into the catalog table.

The upstream sport-id table is mapped to the project's canonical slug
vocabulary in `internal/scraper/scores365/feed.go`. Two slugs deliberately
differ from the upstream names so the project's vocabulary stays
self-consistent:

| SID | Sport | Slug (`events.sport`) | Auto-enrolled? | Notes |
|---|---|---|---|---|
| 1 | Football | `football` | **No** | FotMob owns football; adding it here would steal ~1.5k FotMob leagues. |
| 2 | Basketball | `basketball` | Yes | |
| 3 | Tennis | `tennis` | Yes | |
| 4 | Ice Hockey | `ice-hockey` | Yes | Upstream calls it `hockey`. |
| 5 | Handball | `handball` | Yes | |
| 6 | American Football | `american-football` | Yes | Upstream calls it `a.-football`. |
| 7 | Baseball | `baseball` | Yes | |
| 8 | Volleyball | `volleyball` | Yes | |
| 9 | Rugby | `rugby` | Yes | |

Football SID 1 is intentionally not auto-enrolled. Football leagues can
still be added to `scraper_leagues` manually via
`POST /api/web/v1/scraper-leagues`; if the row's `source='scores365'` and
`override_source` is NULL, the dispatcher routes them through 365scores
regardless.

After PR #135 the dispatcher registers two sources: `fotmob` and
`scores365`. Any `scraper_leagues` row whose `source` is `scores365` goes
through the 365scores source unless `override_source='fotmob'` (or
`'scores365'` to force-pin back if it had been overridden away) is set.

## Discovery

Catalog discovery is implemented in
`internal/scraper/catalog/discovery.go`:

- **One HTTP call** to `client.FetchCompetitions(ctx, sportIDs)` with
  `?sports=id1,id2,...`, against `/web/competitions/`. The 45-second
  timeout (`discoveryTimeout = 45s`) is per call, not per sport — the
  endpoint returns all sports in a single response.
- **Default sport list (`DefaultSportSlugs`)** — eight slugs:
  `basketball, tennis, ice-hockey, handball, american-football, baseball,
  volleyball, rugby`. Football is intentionally omitted; adding it would
  pull ~1.5k competitions that the dispatcher would then have to route
  away from FotMob.
- **Per-row idempotency** — `EnsureLeague` is keyed on
  `(source, source_league_id)`, so re-running discovery against the same
  upstream catalog is a no-op (`upserted`/`unchanged` counters in the log).
  Unknown slugs in `SCORES365_DISCOVERY_SPORTS` are logged as
  `catalog: discovery unknown sport slug` and dropped.
- **Boot-only.** Discovery runs once when the backend starts; there is no
  scheduled re-discovery. To pick up leagues added upstream since the
  last release, restart the backend.

> Note: PR #134 originally implemented discovery by fanning out across
> per-sport `sitemaps/en_<sport>.xml` files. PR #135 superseded that
> with the single `/web/competitions/` request above. Older versions of
> this runbook described the sitemap mechanism — ignore those paragraphs.

## How the cron runs

The schedule is defined in `internal/scheduler/scrape.go`:

- **Today** — `scrapeTodaySpec = "@every 1m"` — every minute, 24/7.
- **Next 7 days** — `scrapeFutureSpec = "0 6,18 * * *"` — daily at
  06:00 and 18:00 UTC.

Each tick:

1. Acquires the Redis distributed lock
   `scheduler:lock:scrape:today` (5-minute TTL,
   `lockScrapeToday = "scheduler:lock:scrape:today"`,
   `ttlScrapeToday = 5m`). The future-tick uses
   `scheduler:lock:scrape:future` with a 25-minute TTL. Locks prevent
   duplicate work across backend instances.
2. Groups the active leagues (those in `scraper_leagues` with
   `enabled=true` and `COALESCE(override_source, source)` matching the
   source) and dispatches each group in parallel. For 365scores this
   calls `DayMatches(date)` on the source, which hits
   `/web/games/allscores/`.
3. The 365scores HTTP client caches `/web/games/allscores/` for 60s per
   UTC date inside `scores365.Client` (`defaultCacheTTL = 60s`), so
   concurrent per-league lookups in the same tick coalesce.
4. New competitions seen in the payload are auto-created via
   `EnsureLeague` (catalog repo upsert) using the **namespaced** league
   id (`LeagueIDPrefix + raw_comp_id = 6_000_000_000 + compId`), so a
   discovered catalog row and the day's matches share a primary key with
   the `tournaments` row they materialize into.

## What to check when something breaks

### "No events today"

1. From the backend host, sanity-check the upstream:
   ```bash
   DAY=$(date -u +%d/%m/%Y)
   curl -i -H 'Referer: https://www.365scores.com/' \
     "https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=$DAY&endDate=$DAY&onlyMajorGames=false&withTop=true"
   ```
   `200` = upstream OK; `503`/`429` = rate-limited or upstream degraded;
   `404` = endpoint moved (escalate). The upstream rejects requests
   without the `Referer` header, so always send it.
2. `docker logs docker-backend-1 --since 5m | grep -i 365scores` (or
   `journalctl -u iptv -n 200`). Look for `fetch error`, `decode error`,
   `429`.
3. On `429`: the HTTP client retries with exponential backoff (1s, 2s,
   4s, 8s, 16s) before giving up. Short 429 bursts within a 60s window
   are absorbed by the in-memory cache; longer bursts exhaust the retries
   and the tick fails. Sustained 429s indicate a real rate-limit —
   escalate before considering a code change.
4. On decode error: the upstream changed the wire format. Capture the
   raw payload with the curl above piped through `head -200`, file a
   bug, and revert to the last working commit.

### "Discovery didn't add the new leagues"

1. Confirm the catalog state:
   ```bash
   docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" scores365 -e \
     "SELECT source, sport, COUNT(*) FROM scraper_leagues WHERE source='scores365' GROUP BY sport;"
   ```
2. If a sport has fewer rows than the previous release, the upstream
   catalog has likely changed shape; check the upstream feed directly
   with the curl above.
3. There is no admin endpoint to re-run discovery. Restart the backend
   (`docker compose restart backend` for compose;
   `sudo systemctl restart iptv.service` for `.deb`).

### "I want to keep using FotMob for NBA"

By default the 365scores source covers NBA (comp 47) via a single bulk
day call, so it's the default. To pin a specific league back to FotMob
without affecting every other 365scores league:

1. Find the NBA row id (`source='scores365'`, `source_league_id` is the
   namespaced id `6000000047`):
   ```bash
   curl -sH "Authorization: Bearer $ADMIN_TOKEN" \
     'https://your.host/api/web/v1/scraper-leagues?source=scores365'
   ```
2. Pin to FotMob:
   ```bash
   curl -X PATCH \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"override_source": "fotmob"}' \
     'https://your.host/api/web/v1/scraper-leagues/<id>'
   ```

The next cron tick routes that league through FotMob. FotMob does not
have a dedicated NBA feed; it resolves NBA matches by per-team or
per-match lookup, which is slower than 365scores' single bulk fetch. Use
this override only if 365scores' NBA coverage is degraded for a sustained
period.

## Date coverage and current limitations

`/web/games/allscores/` takes an explicit `startDate`/`endDate` pair
(`DD/MM/YYYY`), so **arbitrary past and future days are retrievable**.
This is the difference from the legacy `/data/games` feed it replaced:
that one only ever served the current day. The current client
(`FetchGames` in `internal/scores365/client.go`) therefore works for any
date, and `ScrapeNext7Days` gets real data for all seven days.

In practice:

- **One day per request.** The API returns an empty payload when
  `startDate` and `endDate` span more than one day, so the client asks
  for a single day and the scrape loop iterates day by day
  (`ScrapeNext7Days` fetches days +1 … +7 individually).
- **The day boundary follows `SCORES365_TIMEZONE`,** not UTC. The date
  string is derived from the requested timestamp in the client's
  configured zone (`SCORES365_TIMEZONE`, default `UTC`), and the
  upstream stamps each `startTime` with that zone's offset.
- **The future cron (06:00 / 18:00 UTC) does land future fixtures** for
  365scores — a league's next seven days are written to `events` by
  the lookahead job, not only by the per-minute today cron.

If a fuller history is ever needed, the fallback is to scrape HTML
calendars at `https://www.365scores.com/<sport>/<country>/<league>/calendar`
with an HTML parser. That is OUT OF SCOPE for the source package — file
an issue if the need arises.

## Source-language caveat

`SCORES365_LANG_ID` defaults to `29` (es-mx). The cancelled/postponed
keyword classifier (in `internal/scraper/scores365/feed.go`) lists the
es-mx and English variants of the relevant words. Switching to a third
language causes cancelled or postponed fixtures to fall through the
classifier as `finished`, because both `statusGroup=4` and the
localized status text are needed to distinguish them. Leave the default
unless you have a concrete reason to change it and have verified that
the new language's status vocabulary is added to the keyword table.

## Key endpoints

- **Feed (one day):** `GET https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=DD/MM/YYYY&endDate=DD/MM/YYYY&onlyMajorGames=false&withTop=true`
  Required headers:
  ```
  Referer: https://www.365scores.com/
  User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36
  Accept: application/json
  ```
  `startDate` and `endDate` must be the same day — the API returns an
  empty payload for a multi-day range.
- **Catalog (discovery):** `GET https://webws.365scores.com/web/competitions/?sports=2,3,4,5,6,7,8,9`
- **Static assets:** `https://imagecache.365scores.com/image/upload/...`
  Team-logo URLs follow `…/WebSite/team/<CID>/<TeamID>.png`.

## Rollback procedure

If 365scores is causing production issues, two rollback paths exist;
pick the lighter one first.

**Option 1 — pin specific leagues back to FotMob (data-only, instant).**

Use `PATCH /api/web/v1/scraper-leagues/<id>` to set
`override_source='fotmob'` for the affected leagues. This routes only
those leagues away from 365scores; the bulk source keeps running for
unaffected leagues. No restart needed — the next cron tick honors the
new override. See the "I want to keep using FotMob for NBA" procedure
above.

**Option 2 — code rollback (reverts source wiring).**

1. `git revert <merge-sha-of-#135>` on the deployed branch.
2. Restart the backend (`docker compose restart backend` for compose;
   `sudo systemctl restart iptv.service` for `.deb`).

After revert, the dispatcher no longer registers a `scores365` source.
Auto-created `scraper_leagues` rows with `source='scores365'` continue
to exist (the unique key prevents any FotMob-side replacement), but
they are silently skipped — no source matches the name, so no events
are ingested from them. They are dead weight, not corruption.

If you revert both PR #134 (catalog discovery) **and** PR #135 (source
wiring) together, you also lose catalog auto-discovery for 365scores.
Existing catalog rows persist as harmless dead weight either way; no
cleanup is required to keep the system running.

## Related docs

- Source code:
  - `internal/scraper/scores365/` — source adapter, feed adapter, sport-map table
  - `internal/scraper/catalog/discovery.go` — boot-time discovery sweep
  - `internal/scheduler/scrape.go` — cron specs, lock keys, TTLs
  - `internal/scores365/client.go` — HTTP client, `LeagueIDPrefix`, cache TTL
- PRs: #134 (sitemap-based discovery — superseded), #135 (source wiring +
  dispatch), #136 (this runbook)
