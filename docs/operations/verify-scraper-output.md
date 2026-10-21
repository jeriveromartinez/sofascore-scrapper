# Verifying scraper output in production

Queries to confirm that events are actually reaching the database, and
which source produced them. Run against the production MySQL/MariaDB
instance.

## Schema assumptions

The catalog lives in **`scraper_leagues`** (`internal/scraper/catalog/model.go`,
auto-migrated in `internal/platform/database/automigrate.go`):

| Column | Notes |
|---|---|
| `id` | gorm.Model primary key. |
| `source` | The natural source for the league: `fotmob` or `scores365`. |
| `source_league_id` | The league id **as the source uses it**. For 365scores this is the **namespaced** id (`LeagueIDPrefix + raw_comp_id` = `6000000000` + raw comp id), so the catalog row and the materialized `tournaments` row share a primary key. |
| `name`, `country`, `sport` | Display fields. |
| `enabled` | Discovery enrolls rows as `enabled=true`; operators flip off via the admin UI. |
| `override_source` | Optional pointer (`size:32`). When non-NULL, the dispatcher routes this league to that source instead of the natural one. |

The materialized result lives in **`tournaments`** (`internal/tournaments/model.go`):

| Column | Notes |
|---|---|
| `id` | Primary key — equals the namespaced `scraper_leagues.source_league_id`. |
| `name`, `slug`, `region` | Display fields. **No `source` or `sport` column.** |

The per-match fact table is **`events`** (`internal/events/model.go`):

| Column | Notes |
|---|---|
| `id` | gorm.Model primary key. |
| `external_match_id` | Unique across sources; legacy rows backfilled as `legacy-<id>` during `PreMigrateLegacyEvents`. |
| `source` | `varchar(32)`, not NULL. The upstream that produced this event. |
| `sport` | Canonical sport slug (matches `scraper_leagues.sport`). |
| `league_id` | Foreign key into `tournaments.id` — i.e. the **namespaced** league id. |
| `home_score`, `away_score` | Zero for fixtures that haven't kicked off yet, or that the upstream reports as finished with no scoring events. |
| `start_timestamp` | Unix seconds. Zero if extraction failed. |
| `scraped_at` | Unix milliseconds; the per-tick timestamp the scraper assigned. |
| `status_type` | Coarse lifecycle bucket (`scheduled` / `live` / `finished` / `cancelled`). |

Use `events.source` and `events.sport` to trace which upstream and which
sport produced each row. There is no `scraper_leagues` row materialized
into `tournaments`; both rows exist (with shared primary key), so
queries join them by id.

## Diagnostic queries

### 1. Are events being written at all?

A row with thousands of events and a stale timestamp means ingestion
stopped after a successful historical run. An empty result, or a
`MAX(scraped_at)` far in the past, means the scraper stopped entirely —
start with the scraper logs before looking anywhere else.

```sql
SELECT
    source,
    sport,
    COUNT(*)                                  AS events,
    MAX(scraped_at)                           AS last_scrape_ms,
    FROM_UNIXTIME(MAX(scraped_at) / 1000)     AS last_scrape_utc
FROM events
GROUP BY source, sport
ORDER BY events DESC;
```

`last_scrape_utc` should be within one cron interval of `now()` (about a
minute for `scrapeTodaySpec = "@every 1m"`).

### 2. Is one source starving the other?

The multi-source dispatcher fans out to every configured source and
merges the results. If one source returns nothing, the merged output
shrinks silently — the rows from the healthy source look normal in
isolation, so always compare them side by side.

```sql
SELECT
    source,
    COUNT(DISTINCT league_id)                  AS leagues,
    COUNT(*)                                   AS events,
    FROM_UNIXTIME(MAX(start_timestamp) / 1000) AS latest_kickoff_utc
FROM events
GROUP BY source;
```

If a source that was previously present has dropped out of this result
entirely, check the scraper logs for dispatcher warnings — those are the
signals for "source configured but produced nothing" and "league not in
any active source".

### 3. Is the league catalog doing its job?

This query distinguishes **"empty catalog"** (no leagues enrolled) from
**"healthy catalog with no fixtures today"** — the two look identical in
the per-source counts above. A league present in the catalog but missing
in `events` is a matching problem (sport mismatch, stale catalog row,
upstream dropped the comp), which needs different debugging than "the
catalog never enrolled this sport".

`scraper_leagues.source_league_id` is stored as the namespaced string
(e.g. `"6000000047"` for 365scores NBA), and `events.league_id` is an
unsigned integer. Treat them as the same numeric space by casting the
catalog side to unsigned; this works because the seeded catalog rows are
always the namespaced form:

```sql
SELECT
    sl.source,
    sl.sport,
    COUNT(sl.id)                                                  AS catalog_leagues,
    COUNT(e.id)                                                   AS events,
    ROUND(100 * COUNT(e.id) / GREATEST(COUNT(sl.id), 1), 1)       AS pct_materialized
FROM scraper_leagues sl
LEFT JOIN events e
       ON e.league_id = CAST(sl.source_league_id AS UNSIGNED)
      AND e.source   = sl.source
WHERE sl.enabled = 1
GROUP BY sl.source, sl.sport
ORDER BY sl.source, sl.sport;
```

A `pct_materialized` near 100 is expected for `fotmob` on any day with
fixtures; for `scores365` it depends on the day of the year and the
sport's calendar (off-season leagues can legitimately be 0).

If you'd rather inspect the per-league story rather than the summary:

```sql
SELECT
    sl.id            AS catalog_id,
    sl.source,
    sl.source_league_id,
    sl.sport,
    sl.name,
    COUNT(e.id)      AS events
FROM scraper_leagues sl
LEFT JOIN events e
       ON e.league_id = CAST(sl.source_league_id AS UNSIGNED)
      AND e.source   = sl.source
WHERE sl.enabled = 1
  AND sl.source = 'scores365'
GROUP BY sl.id, sl.source, sl.source_league_id, sl.sport, sl.name
ORDER BY events ASC, sl.sport, sl.name
LIMIT 50;
```

A long list of rows with `events = 0` for a sport that's in season is
the canonical signature of "the catalog is healthy but the source isn't
returning fixtures for it."

### 4. Are leagues being materialized into `tournaments`?

A row in `scraper_leagues` and a row in `tournaments` share a primary
key; if one is missing the catalog reference is dangling.

```sql
SELECT
    t.id,
    t.name,
    t.slug,
    t.region,
    COUNT(e.id) AS events
FROM tournaments t
LEFT JOIN events e ON e.league_id = t.id
GROUP BY t.id, t.name, t.slug, t.region
ORDER BY events ASC
LIMIT 50;
```

Leagues with `events = 0` at the bottom of this list are candidates for
removal from the source catalog, or for a sport-mapping bug — a league
configured under the wrong `sport` will never match.

To find catalog rows that have no corresponding `tournaments` row
(dangling references):

```sql
SELECT sl.id, sl.source, sl.source_league_id, sl.sport, sl.name
FROM scraper_leagues sl
LEFT JOIN tournaments t ON t.id = CAST(sl.source_league_id AS UNSIGNED)
WHERE sl.enabled = 1
  AND t.id IS NULL
ORDER BY sl.source, sl.sport, sl.name;
```

### 5. Data quality spot-checks

```sql
-- Finished events with 0-0 score: extraction gap (the source reported
-- "finished" but didn't expose the actual score).
SELECT external_match_id, source, sport, status_type,
       home_score, away_score, scraped_at
FROM events
WHERE status_type = 'finished'
  AND home_score = 0 AND away_score = 0
ORDER BY scraped_at DESC
LIMIT 20;

-- Events with a zero kickoff time: timestamp extraction failed.
SELECT COUNT(*) AS events_without_kickoff
FROM events
WHERE start_timestamp = 0;
```

A non-zero `events_without_kickoff` typically points at timestamp
handling in the source converter rather than at the upstream payload —
the converter zeroes out-of-range values rather than failing.

### 6. Team-logo coverage

The image scheduler downloads upstream logos to local storage and rewrites
the `teams.logo_url` to a relative path served by the backend.

```sql
SELECT
    COUNT(*)                                              AS teams,
    SUM(CASE WHEN logo_url LIKE '/teams/logo/%' THEN 1 ELSE 0 END) AS proxied,
    SUM(CASE WHEN logo_url LIKE 'http%'         THEN 1 ELSE 0 END) AS still_remote
FROM teams;
```

`still_remote` should trend toward zero once the logo scheduler has
caught up. A persistent large value means downloads are failing — check
the `events: failed to download logo for team` log lines (or the
scheduled-job equivalents in your version).
