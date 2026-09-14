# Runbook — 365scores scraper source

**Audience:** on-call operator. **Audience assumes:** basic Go + docker + SQL knowledge.

## Prerequisites

This runbook assumes PRs #134 and #135 have been merged into the deployed branch. Without those:

- The dispatcher only registers `fotmob` and `sportsdb`; no events are scraped for any `scraper_leagues` row with `source='scores365'`.
- The admin UI has no `override_source` field; the documented rollback step below will silently fail.
- The `override_source` PATCH whitelist is missing in the catalog handler.

Verify with:
```bash
git log --oneline -1 | grep -E '#13[45]'
```
If neither PR appears in the recent commits, do NOT follow this runbook — the feature is not yet deployed.

## Post-deploy verification (smoke test)

DB_PASSWORD must be exported (see `.env.example` for the default in dev).

Run after `git pull` on the backend host, BEFORE promoting the change to production:

1. Full reset:
   ```bash
   docker compose -f deployments/docker/compose.dev.yml down -v
   docker compose -f deployments/docker/compose.dev.yml up -d --build backend
   ```
2. Wait 60s for boot + first discovery + cron tick.
3. Verify discovery:
   ```bash
   docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" sofascore -e \
     "SELECT source, sport, COUNT(*) FROM scraper_leagues GROUP BY source, sport ORDER BY source, sport;"
   ```
   Expect: `fotmob` and `scores365` rows; scores365 covers basketball, tennis, hockey, american-football, baseball, volleyball.
4. Verify events:
   ```bash
   docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" sofascore -e \
     "SELECT source, sport, COUNT(*) FROM events GROUP BY source, sport ORDER BY source, sport;"
   ```
5. Hit events API:
   ```bash
   # /api/app/v1/current-events is the registered public endpoint; compose.dev.yml maps host 8080 -> container 8180.
   curl -i http://localhost:8080/api/app/v1/current-events?limit=10
   ```
   Expect HTTP 200, team names populated.
6. Verify a team logo:
   ```bash
   TEAM_ID=$(docker exec docker-mariadb-1 mariadb -uroot -pdevpass1234 sofascore -Nse "SELECT team_id FROM teams ORDER BY team_id DESC LIMIT 1;")
   curl -i "http://localhost:8181/api/app/v1/teams/logo/$TEAM_ID" -o /tmp/logo.png
   file /tmp/logo.png   # expect: PNG image data
   ```

## What 365scores gives us

The 365scores source scrapes `https://webws.365scores.com/data/games`, a public REST endpoint that returns all matches for the current UTC day across 7 sports (the upstream SID table in `internal/scraper/scores365/feed.go`):

| SID | Sport | Sport slug (`events.sport`) | Auto-discovered? | Games/day (approx) |
|---|---|---|---|---|
| 1 | Football | `football` | No (FotMob is primary) | n/a |
| 2 | Basketball | `basketball` | Yes | 16 |
| 3 | Tennis | `tennis` | Yes | 104 |
| 4 | Ice Hockey | `ice-hockey` | Yes | 4 |
| 6 | American Football | `american-football` | Yes | 1 (off-season) |
| 7 | Baseball | `baseball` | Yes | 12 |
| 8 | Volleyball | `volleyball` | Yes | 24 |

Football SID 1 is intentionally NOT auto-discovered (`sportSlugsForDiscovery` in `internal/scraper/catalog/discovery.go`) because FotMob gives richer data for football and is the primary source there. Football leagues can still be added to `scraper_leagues` manually via the admin UI; if their `source` column is `scores365` and `override_source` is NULL, the dispatcher routes them to 365scores.

After merging PR #135 the dispatcher registers two sources: `fotmob` and `scores365`. Any `scraper_leagues` row whose `source` is `scores365` (whether auto-discovered or manually seeded) goes through the 365scores source unless `override_source='fotmob'` is set.

## How the cron runs

The cron schedule is defined in `internal/scheduler/scrape.go`:

- **Today** — `scrapeTodaySpec = "@every 1m"` — every minute, 24/7.
- **Next 7 days (lookahead)** — `scrapeFutureSpec = "0 6,18 * * *"` — daily at 06:00 and 18:00 UTC.

Each tick:

1. Acquires the Redis lock `scheduler:lock:scrape:today` (5-minute TTL, prevents duplicate work across multiple backend instances).
2. Calls `scraper.Service.ScrapeToday`, which groups active leagues by source and dispatches each group in parallel. For 365scores this calls `DayMatches(date)` on the source, which hits `/data/games`.
3. The HTTP layer caches `/data/games` for 60s per UTC date inside the `scores365.Client`, so concurrent per-league lookups in the same tick don't hit the network twice.
4. New competitions seen in the payload are auto-created via `EnsureLeague` (catalog repo upsert).

Discovery runs **once at boot only** (30s timeout per sport, 6 sports total). It reads `sitemaps/en_<sport>.xml` and upserts new leagues into `scraper_leagues` with `source='scores365'`. There is no scheduled weekly re-discovery; if you need to re-pick-up leagues added upstream since the last release, restart the backend.

## What to check when something breaks

### "No events today"

1. Check `curl -i https://webws.365scores.com/data/games?lang=en` from the backend host. 200 = upstream OK; 503/429 = rate-limited or upstream degraded; 404 = endpoint removed (escalate).
2. `docker logs docker-backend-1 --since 5m | grep -i scores365`. Look for `fetch error`, `decode error`, or `429`.
3. If 429: The HTTP client retries with exponential backoff (1s, 2s, 4s, 8s, 16s) before giving up. Short 429 bursts within a 60s window are absorbed by the in-memory cache; longer bursts exhaust the retries and the tick fails. Sustained 429s indicate a real rate-limit; escalate before considering a code change.
4. If decode error: the upstream changed the wire format. Capture the response with `curl 'https://webws.365scores.com/data/games?lang=en' | head -200`, file a bug, and revert to the last working commit.

### "Discovery didn't add new leagues"

1. `docker exec docker-mariadb-1 mariadb -uroot -p"${DB_PASSWORD}" sofascore -e "SELECT source, sport, COUNT(*) FROM scraper_leagues WHERE source='scores365' GROUP BY sport;"`
2. If a sport has fewer rows than the previous release, the sport sitemap may have been moved or renamed.
3. `curl -I https://www.365scores.com/sitemaps/en_basketball.xml` — 200 means reachable. (Substitute the sport slug: basketball, tennis, hockey, american-football, baseball, volleyball.)
4. There is no admin endpoint to re-run discovery; restart the backend container (`docker compose restart backend` for compose; `sudo systemctl restart iptv.service` for `.deb`).

### "I want to keep using FotMob for NBA"

The 365scores feed covers NBA via a single bulk day-wide call (comp 47 — NBA), so 365scores is the default. If you want to pin a specific league back to FotMob:

1. Go to `web/src/admin/scraper-leagues`.
2. Find the NBA row (`source='scores365'`, `source_league_id='47'`).
3. Click Edit.
4. Override source = `fotmob`.
5. Save.

The next cron tick will route that league through FotMob instead. Note: FotMob does not have a dedicated NBA feed; it resolves NBA matches by per-team or per-match lookup, which is slower than 365scores' single bulk fetch. Use this only if 365scores' NBA coverage is degraded for a sustained period.

## Limitations (and the workaround we accepted)

The `/data/games` endpoint only returns the **current UTC day** — past and future days are NOT retrievable through this endpoint. As a result:

- **Yesterday's matches**: present until ~midnight UTC, then gone.
- **Tomorrow's matches**: appear at midnight UTC on the day-of (the cron will pick them up as "today" then).
- **The `ScrapeNext7Days` lookahead cron is effectively a no-op for 365scores** — it requests day N+1 but the upstream returns day N regardless. The cron still runs (every minute via `scrapeTodaySpec` and 06:00/18:00 UTC via `scrapeFutureSpec`), but only today's matches land in `events`.
- If a date-capable endpoint becomes available, the runbook should be updated; today, document this limitation and rely on the per-minute today cron for full coverage.

If we ever need a fuller history, the fallback plan is to scrape `https://www.365scores.com/<sport>/<country>/<league>/calendar` (HTML parser). That is OUT OF SCOPE for this PR — file an issue if a need arises.

## Key endpoints

- **Feed:** `GET https://webws.365scores.com/data/games?lang=en`
  Headers: `Referer: https://www.365scores.com/`, `User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`, `Accept: application/json`
- **Sitemaps:** `https://www.365scores.com/sitemaps/<lang>_<sport>.xml`
  Sports covered by auto-discovery: basketball, tennis, hockey, american-football, baseball, volleyball. (Football is omitted from auto-discovery; see "What 365scores gives us" above.)
- **Static assets:** `https://imagecache.365scores.com/image/upload/...`
  Logo URLs follow `…/WebSite/team/<CID>/<TeamID>.png` (verify against current schema if logos break).

## Rollback procedure

If 365scores is causing production issues:

1. Set `override_source='fotmob'` for the affected leagues in `scraper_leagues` via the admin UI (per the "I want to keep using FotMob for NBA" procedure above). This routes the affected leagues away from 365scores immediately; the bulk source keeps running for unaffected leagues.
2. For a code-level rollback, revert PR #135 (`git revert <merge-sha>`).
3. Restart the backend (`docker compose restart backend` for compose; `sudo systemctl restart iptv.service` for `.deb`).

Note: reverting PR #135 also reverts the auto-discovery changes from PR #134 if you do it on the same branch. To roll back only the source wiring (the dispatcher registration + the `scores365` source impl) without losing discovery, revert PR #135 first, then leave PR #134 in place. The auto-created `scraper_leagues` rows with `source='scores365'` will continue to exist post-revert but the dispatcher will skip them (no source implements that name after revert), so no events will be ingested from those rows.

## Related docs

- Spec: `docs/superpowers/specs/2026-09-13-scores365-source-design.md`
- Implementation plan: `docs/superpowers/plans/2026-09-13-scores365-source.md`
- PRs: #134 (sitemap-based discovery), #135 (source wiring + dispatch), #136 (this runbook)