// Package sportsdb implements a client for TheSportsDB public API.
//
// TheSportsDB (https://www.thesportsdb.com/api.php) exposes a free,
// no-auth search endpoint that returns badge / logo URLs for football
// clubs worldwide. We use it as a fallback for the LogoScheduler
// when the SofaScore CDN returns 404 — FotMob and SofaScore maintain
// independent team-ID spaces, so a FotMob-sourced team like Juventus
// (team_id=9885) maps to nothing on SofaScore CDN's
// /api/v1/team/9885/image, but TheSportsDB has it under "Juventus".
//
// The free API uses the public key "3". Patreon supporters can pass a
// personal key via the THESPORTSDB_API_KEY env var. We cache successful
// name→badge-URL lookups for 24h to stay well under the public rate
// limit (~30 req/min) — a daily cron only needs one resolution per
// team.
package sportsdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultAPIKey is the public TheSportsDB key documented at
	// https://www.thesportsdb.com/api.php. Patreon supporters can
	// pass their personal key via WithAPIKey.
	DefaultAPIKey = "3"

	// DefaultBaseURL is the TheSportsDB v1 API base URL.
	DefaultBaseURL = "https://www.thesportsdb.com/api/v1/json"

	// DefaultTimeout caps each upstream call.
	DefaultTimeout = 5 * time.Second

	// cacheTTL is how long a name→badge-URL mapping stays in the
	// in-memory cache. 24h is well above the daily cron interval
	// and keeps us far below the public API rate limit.
	cacheTTL = 24 * time.Hour

	// negativeCacheTTL is how long a "not found" result stays in
	// the cache. Negative hits are short-lived so a transient
	// upstream outage doesn't permanently mask a team.
	negativeCacheTTL = 15 * time.Minute

	// rateLimitInterval is the minimum gap between successive
	// upstream requests. TheSportsDB's free tier allows ~30
	// requests/minute, so a 5s gap (= 12 req/min) leaves ample
	// headroom under the documented quota. Empirically the
	// free-tier API enforces stricter anti-abuse than 30/min —
	// bursts at the documented limit return 429 — so a
	// conservative gap keeps us below the threshold.
	rateLimitInterval = 5 * time.Second

	// throttleBackoff is how long waitForRateLimit refuses to
	// forward upstream calls after a 429. Set to 5 minutes
	// because TheSportsDB's anti-abuse lockout appears to
	// persist beyond the documented per-minute quota.
	throttleBackoff = 5 * time.Minute

	// User-Agent identifies our scraper in TheSportsDB logs.
	UserAgent = "sofascore-scrapper/1.0 (logo-lookup)"
)

// Client looks up football team logos via TheSportsDB v1 API.
//
// It is safe for concurrent use. Lookups are cached in-memory under a
// mutex-guarded map keyed by lower-cased team name; entries expire
// after cacheTTL. Negative results (name not found, request error)
// are cached for a shorter window so transient upstream failures heal
// on the next cron tick.
//
// All lookups share a single in-process rate limiter (one request
// every rateLimitInterval). Without it the 10-worker LogoScheduler
// would burst 10 simultaneous upstream calls on the first scrape
// and trip TheSportsDB's free-tier limit (~30 req/min).
//
// When the upstream responds with HTTP 429 (Too Many Requests) the
// rate limiter is paused for throttleBackoff. This matches
// TheSportsDB's observed anti-abuse behaviour: a single burst of
// 30+ requests in the first 30s of a new instance triggers a
// multi-minute lock-out that is longer than the documented
// per-minute quota.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client

	mu        sync.Mutex
	cache     map[string]cachedEntry
	rateMu    sync.Mutex
	lastFetch time.Time
	throttleMu sync.Mutex
	throttleUntil time.Time

	// dayCache holds EventsByDay results keyed by league+date.
	dayCache eventsByDayCacheMap

	// testRateInterval overrides rateLimitInterval when set by
	// NewClientForTest. Production code leaves it at zero and the
	// waitForRateLimit method falls back to rateLimitInterval.
	testRateInterval time.Duration
	// testThrottleBackoff overrides throttleBackoff when set by
	// NewClientForTest. Production code leaves it at zero and the
	// recordThrottle method falls back to throttleBackoff.
	testThrottleBackoff time.Duration
}

// Options configures a Client. Zero values pick defaults.
type Options struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

type cachedEntry struct {
	badgeURL string
	leagues  []League
	events   []Event
	err      error
	expires  time.Time
}

// Event is a single match surfaced by EventsByDay. The shape is a
// straight pass-through of the TheSportsDB eventsday.php payload —
// the scraper source layer maps this to scraper.Match.
type Event struct {
	IDEvent   string
	IDAPI     string
	IDLeague  string
	HomeTeam  string
	AwayTeam  string
	HomeScore int
	AwayScore int
	// IDHomeTeam and IDAwayTeam are the TheSportsDB team IDs
	// (e.g. "134923" for Cincinnati Bengals). Empty when the
	// upstream omits the field (older fixtures, teams still
	// being indexed). The scraper namespace-prefixes these so
	// they don't collide with FotMob team IDs in the shared
	// teams table.
	IDHomeTeam string
	IDAwayTeam string
	// HomeTeamBadge and AwayTeamBadge are the TheSportsDB badge
	// URLs (e.g. https://r2.thesportsdb.com/images/media/team/
	// badge/...). Empty when the upstream has no badge yet.
	HomeTeamBadge string
	AwayTeamBadge string
	// Timestamp is the wall-clock UTC for the match kick-off,
	// derived from dateEvent + strTime + strTimestamp (the latter
	// wins when present so the upstream's authoritative value
	// survives).
	Timestamp time.Time
	// Postponed is true when the upstream emitted
	// strPostponed:"yes". The scraper treats postponed matches
	// as cancelled and drops them from the daily upsert.
	Postponed bool
	// League and Sport pass through from the upstream payload
	// ("NBA", "Basketball") so the scraper can detect mapping
	// errors during field renames.
	League  string
	Sport   string
	Country string
}

// eventsByDayCacheEntry stores the parsed events under the cache
// key (leagueID, date). The same cache TTL applies — 24h for hits,
// 15min for misses — so a transient outage doesn't permanently
// mask a day.
type eventsByDayCacheEntry struct {
	events  []Event
	err     error
	expires time.Time
}

type eventsByDayCacheMap struct {
	mu sync.Mutex
	m  map[string]eventsByDayCacheEntry
}

// League is the shape of a single entry from TheSportsDB's
// all_leagues.php endpoint. We use it for catalog discovery: the
// scraper pulls every league the upstream publishes so the admin
// catalog reflects the full set without manual curation.
//
// Sport is TheSportsDB's verbose display form ("American Football",
// "Ice Hockey"). The scraper maps these to canonical lowercase
// sport strings via NormalizeSport before persisting to
// scraper_leagues.sport.
//
// Country is empty when the upstream omits it (some cup
// competitions and international tournaments). We persist the
// empty string rather than a placeholder.
type League struct {
	ID      string
	Name    string
	Sport   string
	Country string
}

// allLeaguesResponse wraps the upstream all_leagues.php envelope.
type allLeaguesResponse struct {
	Leagues []apiLeague `json:"leagues"`
}

// apiLeague is the upstream wire shape. We only decode the four
// fields we need; everything else (strBadge, strDescriptionEN, etc.)
// is dropped at unmarshal time to keep the cache footprint small.
type apiLeague struct {
	IDLeague   string `json:"idLeague"`
	StrLeague  string `json:"strLeague"`
	StrSport   string `json:"strSport"`
	StrCountry string `json:"strCountry"`
}

// AllLeagues every league TheSportsDB publishes, deduplicated by
// idLeague.
//
// The free tier of the upstream returns at most ~10 entries on
// this endpoint per request — the published limit on the docs
// page is "10 free" — so the result set is truncated by the
// upstream. Callers must treat the slice as "top of the catalog"
// rather than "the whole catalog". The full population happens
// via eventsday.php sweep (see scraper.DayMatches: every event
// carries its idLeague, so a single day-wide fetch enumerates
// every league with matches on that date).
//
// Cached for cacheTTL (24h) — the list rarely changes day-to-day.
// Negative hits (transient upstream errors) are cached for
// negativeCacheTTL so a 429 today doesn't permanently mask the
// catalog after the throttle window lifts.
func (c *Client) AllLeagues(ctx context.Context) ([]League, error) {
	cacheKey := "all:leagues"
	if entry, ok := c.lookupCache(cacheKey); ok {
		if entry.err != nil {
			return nil, entry.err
		}
		return entry.leagues, nil
	}
	leagues, err := c.fetchAllLeagues(ctx)
	ttl := cacheTTL
	if err != nil {
		ttl = negativeCacheTTL
	}
	c.storeLeagues(cacheKey, leagues, err, ttl)
	if err != nil {
		return nil, err
	}
	return leagues, nil
}

// fetchAllLeagues hits all_leagues.php once and parses the result.
// Rate-limited through the shared client limiter.
func (c *Client) fetchAllLeagues(ctx context.Context) ([]League, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("sportsdb: rate limit wait: %w", err)
	}
	u := fmt.Sprintf("%s/%s/all_leagues.php", c.baseURL, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: build all_leagues request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: all_leagues request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		c.recordThrottle()
		return nil, fmt.Errorf("sportsdb: all_leagues returned 429")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sportsdb: all_leagues returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: read all_leagues body: %w", err)
	}
	var parsed allLeaguesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sportsdb: decode all_leagues: %w", err)
	}
	out := make([]League, 0, len(parsed.Leagues))
	for _, raw := range parsed.Leagues {
		if raw.IDLeague == "" {
			continue
		}
		out = append(out, League{
			ID:      raw.IDLeague,
			Name:    raw.StrLeague,
			Sport:   raw.StrSport,
			Country: raw.StrCountry,
		})
	}
	return out, nil
}

// NormalizeSport maps TheSportsDB's verbose display sport name
// ("American Football", "Ice Hockey", "Soccer") to the canonical
// lowercase form the rest of the system uses ("american-football",
// "ice-hockey", "football"). When the upstream introduces a sport
// the table does not yet know about, we return a heuristic
// lowercased, dashed form so the catalog stays stable until an
// admin renames it.
//
// The mapping is intentionally a function (not a constant map) so
// future sports can extend it without breaking callers.
func NormalizeSport(display string) string {
	switch strings.ToLower(strings.TrimSpace(display)) {
	case "":
		return ""
	case "soccer":
		return "football"
	case "american football":
		return "american-football"
	case "ice hockey":
		return "ice-hockey"
	}
	// Fallback: lowercase, spaces → dashes.
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(display)), " ", "-")
}// NewClient returns a Client configured with the supplied Options.
// Missing fields fall back to package defaults.
func NewClient(opts Options) *Client {
	apiKey := opts.APIKey
	if apiKey == "" {
		apiKey = DefaultAPIKey
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	return &Client{
		apiKey:   apiKey,
		baseURL:  baseURL,
		http:     httpClient,
		cache:    make(map[string]cachedEntry),
		dayCache: eventsByDayCacheMap{m: make(map[string]eventsByDayCacheEntry)},
	}
}

// NewClientForTest is identical to NewClient except it uses a
// caller-supplied rate-limit interval instead of rateLimitInterval.
// Tests use this to verify rate-limit behaviour in milliseconds
// rather than waiting for the production 2-second gap.
func NewClientForTest(opts Options, interval time.Duration) *Client {
	c := NewClient(opts)
	c.testRateInterval = interval
	return c
}

// apiTeam is the subset of TheSportsDB's team payload we consume.
// The upstream returns ~30 fields; we only decode the ones that drive
// logo selection.
type apiTeam struct {
	IDTeam      string `json:"idTeam"`
	StrTeam     string `json:"strTeam"`
	StrBadge    string `json:"strBadge"`
	StrLogo     string `json:"strLogo"`
	StrLeague   string `json:"strLeague"`
	StrCountry  string `json:"strCountry"`
	StrSport    string `json:"strSport"`
}

type searchResponse struct {
	Teams []apiTeam `json:"teams"`
}

// TeamLogoURL returns the badge URL for the team that best matches the
// passed name. The first match wins; we prefer strBadge (which is the
// small badge variant) and fall back to strLogo (which is a larger logo
// variant) when strBadge is empty.
//
// The lookup is cached in-memory. A non-nil error indicates the team
// was not found in TheSportsDB or the upstream call failed — both
// are reported via a non-empty error and (separately) an empty URL.
//
// Example:
//
//	url, err := c.TeamLogoURL(ctx, "Juventus")
//	// url = "https://r2.thesportsdb.com/images/media/team/badge/uxf0gr1742983727.png"
func (c *Client) TeamLogoURL(ctx context.Context, name string) (string, error) {
	key := normalizeTeamName(name)
	if key == "" {
		return "", fmt.Errorf("sportsdb: empty team name")
	}

	if entry, ok := c.lookupCache(key); ok {
		return entry.badgeURL, entry.err
	}

	badgeURL, err := c.fetchTeamLogo(ctx, name)
	c.storeCache(key, badgeURL, err)
	return badgeURL, err
}

// normalizeTeamName collapses whitespace and lowercases so two
// callers asking about "Juventus" and "  juventus " hit the same
// cache slot.
func normalizeTeamName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (c *Client) lookupCache(key string) (cachedEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[key]
	if !ok || time.Now().After(entry.expires) {
		return cachedEntry{}, false
	}
	return entry, true
}

func (c *Client) storeCache(key, badgeURL string, err error) {
	ttl := cacheTTL
	if err != nil {
		// Negative results get a shorter TTL so a transient
		// outage doesn't permanently mask a team.
		ttl = negativeCacheTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cachedEntry{
		badgeURL: badgeURL,
		err:      err,
		expires:  time.Now().Add(ttl),
	}
}

// storeLeagues writes a leagues-shaped cache entry. Called from
// AllLeagues where the cached value is the full list rather than
// a single badge URL. The err argument is recorded alongside the
// leagues so a cached 429 from a previous call short-circuits the
// next caller instead of going back to the upstream.
func (c *Client) storeLeagues(key string, leagues []League, err error, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cachedEntry{
		leagues: leagues,
		err:     err,
		expires: time.Now().Add(ttl),
	}
}

// waitForRateLimit blocks until the next upstream call is allowed.
// It honours ctx cancellation so a shutdown does not stall on a
// multi-second rate-limit wait.
//
// TheSportsDB's free tier allows ~30 requests/minute; without this
// guard, a 10-worker scheduler burst on first run trips HTTP 429
// for every lookup. The limiter is in-process — multiple backend
// instances each get their own quota. That is acceptable because
// the daily cron only re-fetches teams whose cache entry expired.
func (c *Client) waitForRateLimit(ctx context.Context) error {
	if remaining := c.throttleRemaining(); remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	interval := rateLimitInterval
	if c.testRateInterval > 0 {
		interval = c.testRateInterval
	}
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	wait := time.Until(c.lastFetch.Add(interval))
	if wait <= 0 {
		c.lastFetch = time.Now()
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		c.lastFetch = time.Now()
		return nil
	}
}

func (c *Client) fetchTeamLogo(ctx context.Context, name string) (string, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return "", fmt.Errorf("sportsdb: rate limit wait: %w", err)
	}
	u := fmt.Sprintf("%s/%s/searchteams.php?t=%s", c.baseURL, c.apiKey, url.QueryEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("sportsdb: build request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("sportsdb: fetch %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		// Pause the rate limiter for the rest of the current minute.
		// TheSportsDB documents its throttling as a per-minute quota
		// (https://www.thesportsdb.com/api.php), so the next call
		// after the wall clock crosses the minute boundary should
		// succeed.
		c.recordThrottle()
		return "", fmt.Errorf("sportsdb: %s: unexpected HTTP 429", name)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sportsdb: %s: unexpected HTTP %d", name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("sportsdb: read body: %w", err)
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("sportsdb: decode payload: %w", err)
	}
	if len(parsed.Teams) == 0 {
		return "", fmt.Errorf("sportsdb: no match for %q", name)
	}

	for _, t := range parsed.Teams {
		if t.StrBadge != "" {
			return t.StrBadge, nil
		}
		if t.StrLogo != "" {
			return t.StrLogo, nil
		}
	}
	return "", fmt.Errorf("sportsdb: %s: matched but no badge/logo URL", name)
}

// EventsByDay returns the events scheduled for `date` in the
// TheSportsDB league identified by `leagueID`. The date is passed
// through verbatim so callers can use any timezone they want —
// the upstream expects YYYY-MM-DD strings.
//
// Results are cached in-memory under (leagueID, date) with the
// same TTLs as TeamLogoURL (24h for hits, 15min for misses). The
// shared rate limiter prevents the daily multi-league cron from
// bursting through the free-tier 30 req/min quota.
//
// An empty result ({"events":null}) is a valid empty day, not an
// error — the scheduler treats it as a no-op.
func (c *Client) EventsByDay(ctx context.Context, leagueID string, date string) ([]Event, error) {
	if leagueID == "" {
		return nil, fmt.Errorf("sportsdb: empty league id")
	}
	if date == "" {
		return nil, fmt.Errorf("sportsdb: empty date")
	}

	if cached, ok := c.lookupDayCache(leagueID, date); ok {
		return cached.events, cached.err
	}

	events, err := c.fetchDayEvents(ctx, leagueID, date)
	c.storeDayCache(leagueID, date, events, err)
	return events, err
}

// EventsDayAll fetches every event TheSportsDB publishes for the
// given date — across every sport, every league, every match.
// Internally this is the same eventsday.php endpoint as
// EventsByDay, but without the `l=<leagueID>` filter the
// upstream returns the day's full slate in a single round-trip.
//
// The free-tier rate limit on this endpoint is 3 req/min; with
// the per-call 5s gap the shared client limiter produces one
// fetch per minute, well inside the budget.
//
// Returns an empty slice when the day has no fixtures
// ({"events":null} on the wire). Errors are cached briefly so
// a tight retry loop does not hammer a throttled upstream.
func (c *Client) EventsDayAll(ctx context.Context, date string) ([]Event, error) {
	if date == "" {
		return nil, fmt.Errorf("sportsdb: empty date")
	}
	cacheKey := "all:" + date
	if entry, ok := c.lookupCache(cacheKey); ok {
		return entry.events, entry.err
	}
	events, err := c.fetchDayEventsAll(ctx, date)
	ttl := cacheTTL
	if err != nil {
		ttl = negativeCacheTTL
	}
	c.storeAllDayCache(cacheKey, events, err, ttl)
	return events, err
}

// fetchDayEventsAll hits eventsday.php?d=<date> (no league
// filter) and parses the full-day payload. Rate-limited through
// the shared client limiter.
func (c *Client) fetchDayEventsAll(ctx context.Context, date string) ([]Event, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("sportsdb: rate limit wait: %w", err)
	}
	u := fmt.Sprintf("%s/%s/eventsday.php?d=%s",
		c.baseURL, c.apiKey, url.QueryEscape(date))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: build eventsday request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: eventsday request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		c.recordThrottle()
		return nil, fmt.Errorf("sportsdb: eventsday returned 429")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sportsdb: eventsday returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: read eventsday body: %w", err)
	}
	var parsed dayEventsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sportsdb: decode eventsday: %w", err)
	}
	out := make([]Event, 0, len(parsed.Events))
	for _, raw := range parsed.Events {
		out = append(out, decodeDayEvent(raw))
	}
	return out, nil
}

// storeAllDayCache writes an eventsday-shaped cache entry under
// the day-wide key. Reuses the existing cachedEntry struct so
// the cache layer needs no new types.
func (c *Client) storeAllDayCache(key string, events []Event, err error, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cachedEntry{
		events:  events,
		err:     err,
		expires: time.Now().Add(ttl),
	}
}

func (c *Client) lookupDayCache(leagueID, date string) (eventsByDayCacheEntry, bool) {
	c.dayCache.mu.Lock()
	defer c.dayCache.mu.Unlock()
	entry, ok := c.dayCache.m[leagueID+"|"+date]
	if !ok || time.Now().After(entry.expires) {
		return eventsByDayCacheEntry{}, false
	}
	return entry, true
}

func (c *Client) storeDayCache(leagueID, date string, events []Event, err error) {
	ttl := cacheTTL
	if err != nil {
		ttl = negativeCacheTTL
	}
	c.dayCache.mu.Lock()
	defer c.dayCache.mu.Unlock()
	c.dayCache.m[leagueID+"|"+date] = eventsByDayCacheEntry{
		events:  events,
		err:     err,
		expires: time.Now().Add(ttl),
	}
}

// dayEvent is the wire shape of a single event in the
// eventsday.php payload. We only decode the fields we need.
type dayEvent struct {
	IDEvent       string `json:"idEvent"`
	IDAPI         string `json:"idAPIfootball"`
	IDLeague      string `json:"idLeague"`
	HomeTeam      string `json:"strHomeTeam"`
	AwayTeam      string `json:"strAwayTeam"`
	HomeScore     string `json:"intHomeScore"`
	AwayScore     string `json:"intAwayScore"`
	DateEvent     string `json:"dateEvent"`
	StrTime       string `json:"strTime"`
	StrTimeLocal  string `json:"strTimeLocal"`
	StrTimestamp  string `json:"strTimestamp"`
	League        string `json:"strLeague"`
	Sport         string `json:"strSport"`
	Country       string `json:"strCountry"`
	Postponed     string `json:"strPostponed"`
	IDHomeTeam    string `json:"idHomeTeam"`
	IDAwayTeam    string `json:"idAwayTeam"`
	HomeTeamBadge string `json:"strHomeTeamBadge"`
	AwayTeamBadge string `json:"strAwayTeamBadge"`
}

type dayEventsResponse struct {
	Events []dayEvent `json:"events"`
}

func (c *Client) fetchDayEvents(ctx context.Context, leagueID, date string) ([]Event, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("sportsdb: rate limit wait: %w", err)
	}
	u := fmt.Sprintf("%s/%s/eventsday.php?d=%s&l=%s",
		c.baseURL, c.apiKey, url.QueryEscape(date), url.QueryEscape(leagueID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: build request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: fetch day events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.recordThrottle()
		return nil, fmt.Errorf("sportsdb: day events %s: unexpected HTTP 429", date)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sportsdb: day events %s: unexpected HTTP %d", date, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("sportsdb: read body: %w", err)
	}

	// TheSportsDB returns {"events":null} on empty days, not
	// {"events":[]}. We unmarshal into a slice and treat nil as
	// an empty result so callers see a consistent []Event{}.
	var parsed dayEventsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sportsdb: decode day events: %w", err)
	}
	out := make([]Event, 0, len(parsed.Events))
	for _, raw := range parsed.Events {
		out = append(out, decodeDayEvent(raw))
	}
	return out, nil
}

func decodeDayEvent(raw dayEvent) Event {
	ts := parseEventTimestamp(raw)
	home, _ := strconv.Atoi(raw.HomeScore)
	away, _ := strconv.Atoi(raw.AwayScore)
	return Event{
		IDEvent:        raw.IDEvent,
		IDAPI:          raw.IDAPI,
		IDLeague:       raw.IDLeague,
		HomeTeam:       raw.HomeTeam,
		AwayTeam:       raw.AwayTeam,
		HomeScore:      home,
		AwayScore:      away,
		IDHomeTeam:     raw.IDHomeTeam,
		IDAwayTeam:     raw.IDAwayTeam,
		HomeTeamBadge:  raw.HomeTeamBadge,
		AwayTeamBadge:  raw.AwayTeamBadge,
		Timestamp:      ts,
		Postponed:      strings.EqualFold(raw.Postponed, "yes"),
		League:         raw.League,
		Sport:          raw.Sport,
		Country:        raw.Country,
	}
}

// parseEventTimestamp prefers strTimestamp (RFC3339) when the
// upstream supplies it, falling back to dateEvent + strTime. We
// always emit UTC so callers don't have to track the league's
// reporting timezone.
func parseEventTimestamp(raw dayEvent) time.Time {
	if t, err := time.Parse(time.RFC3339, raw.StrTimestamp); err == nil {
		return t.UTC()
	}
	if raw.DateEvent != "" && raw.StrTime != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04:05",
			raw.DateEvent+" "+raw.StrTime, time.UTC)
		if err == nil {
			return t.UTC()
		}
	}
	if raw.DateEvent != "" {
		t, err := time.ParseInLocation("2006-01-02", raw.DateEvent, time.UTC)
		if err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
// recordThrottle pauses the rate limiter for throttleBackoff from
// now. Subsequent calls to waitForRateLimit will sleep until that
// point instead of returning immediately. Multiple 429s within the
// same backoff window extend the deadline each time, so a sustained
// burst keeps the limiter shut.
func (c *Client) recordThrottle() {
	backoff := throttleBackoff
	if c.testThrottleBackoff > 0 {
		backoff = c.testThrottleBackoff
	}
	c.throttleMu.Lock()
	defer c.throttleMu.Unlock()
	reset := time.Now().Add(backoff)
	if reset.After(c.throttleUntil) {
		c.throttleUntil = reset
	}
}

// throttleRemaining reports how long until the next wall-clock minute
// boundary (zero when not currently throttled).
func (c *Client) throttleRemaining() time.Duration {
	c.throttleMu.Lock()
	defer c.throttleMu.Unlock()
	return time.Until(c.throttleUntil)
}
