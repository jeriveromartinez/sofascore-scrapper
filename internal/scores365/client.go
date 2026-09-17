// Package scores365 implements an HTTP client for the public
// 365scores.com web API used to discover competitions and fetch
// multi-sport fixtures.
//
// Everything this package reads comes from the JSON API that
// www.365scores.com itself calls; no HTML or XML markup is parsed.
// Four endpoints are used, all on https://webws.365scores.com:
//
//   - /web/games/allscores/ — every fixture for a given calendar day
//     across every sport, together with the competition and country
//     dictionaries needed to resolve them (~825KB raw, ~130KB gzipped).
//   - /web/competitions/    — the full competition catalog. Passing
//     ?sports=1,2,3,… is required to get more than the handful of
//     "popular" competitions returned by default (~2.7k rows).
//   - /web/sports/          — the sport dictionary (id → nameForURL).
//   - /web/countries/       — the country dictionary.
//
// Every request carries the shared query parameters the web app
// sends: appTypeId (5 = web), langId (29 = es-mx), timezoneName and
// userCountryId. The site gates requests behind a Referer check, so
// every outgoing request also sets Referer: https://www.365scores.com/
// and a desktop Chrome User-Agent. Responses are cached in memory per
// request URL for a short TTL; on 429 and 5xx we back off 1s, 2s, 4s,
// 8s, 16s before giving up.
//
// Note on compression: Accept-Encoding is deliberately left unset so
// net/http negotiates gzip transparently and decompresses the body
// for us (825KB → 130KB on the wire).
package scores365

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
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	Referer   = "https://www.365scores.com/"

	// DefaultBaseURL is the JSON API host the 365scores web app calls.
	DefaultBaseURL = "https://webws.365scores.com"
	// DefaultImageBaseURL is the Cloudinary-backed image CDN that
	// serves competitor, competition and country badges.
	DefaultImageBaseURL = "https://imagecache.365scores.com"

	// DefaultAppTypeID identifies the web client (5). The API returns
	// truncated payloads for unknown app types.
	DefaultAppTypeID = 5
	// DefaultLangID is es-mx, matching https://www.365scores.com/es-mx.
	// Competition, competitor and status labels come back localized in
	// this language.
	DefaultLangID = 29
	// DefaultUserCountryID scopes the "popularity" ordering of the
	// payload; 116 is what the es-mx site sends. It does not filter
	// the result set.
	DefaultUserCountryID = 116
	// DefaultTimezoneName is the IANA zone used to bucket fixtures
	// into calendar days. Game.StartTime is returned with this zone's
	// UTC offset, so the day boundary of the feed follows it.
	DefaultTimezoneName = "UTC"
)

// LeagueIDPrefix namespaces 365scores competition IDs into the
// shared `tournaments` table. 365scores uses small numeric comp IDs
// (e.g. 47) that collide with FotMob league IDs (e.g. 47 for the
// English Premier League). Prefixing every row with 6_000_000_000
// puts the 365scores range well above the natural FotMob range;
// persistence via ToTournament then uses the prefixed ID as the row
// primary key, so the two sources can never overwrite each other.
//
// The same prefix is applied on the catalog discovery path so
// scraper_leagues rows seeded from /web/competitions/ and tournaments
// rows seeded from /web/games/allscores/ land on the same ID space.
const LeagueIDPrefix int64 = 6_000_000_000

// defaultCacheTTL keeps the once-a-minute scrape loop from
// re-fetching the same day twice within a single pass.
const defaultCacheTTL = 60 * time.Second

// Logger is the minimal logging surface the client needs. *slog.Logger
// satisfies it.
type Logger interface {
	Warn(msg string, args ...any)
}

type Options struct {
	BaseURL       string
	ImageBaseURL  string
	AppTypeID     int
	LangID        int
	TimezoneName  string
	UserCountryID int
	// SportIDs is the default sport filter for FetchCompetitions.
	// It is ignored by FetchGames — the allscores feed always
	// returns every sport.
	SportIDs   []int
	CacheTTL   time.Duration
	HTTPClient *http.Client
	Logger     Logger
}

type Client struct {
	baseURL       string
	imageBaseURL  string
	appTypeID     int
	langID        int
	timezoneName  string
	userCountryID int
	sportIDs      []int
	cacheTTL      time.Duration
	http          *http.Client
	referer       string
	log           Logger

	cacheMu sync.Mutex
	cache   map[string]cacheEntry // key = request URL
}

type cacheEntry struct {
	feed      *Feed
	expiresAt time.Time
}

// Feed is the common envelope every /web/ endpoint returns. Each
// endpoint fills in only the slices it owns: allscores populates
// Games plus the Sports/Countries/Competitions dictionaries,
// /web/competitions/ populates Competitions (with the Sports and
// Countries dictionaries), /web/sports/ populates Sports.
type Feed struct {
	LastUpdateID      int64         `json:"lastUpdateId"`
	RequestedUpdateID int64         `json:"requestedUpdateId"`
	TTL               int           `json:"ttl"`
	Sports            []Sport       `json:"sports"`
	Countries         []Country     `json:"countries"`
	Competitions      []Competition `json:"competitions"`
	Competitors       []Competitor  `json:"competitors"`
	Games             []Game        `json:"games"`
}

type Sport struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	NameForURL   string `json:"nameForURL"`
	DrawSupport  bool   `json:"drawSupport"`
	ImageVersion int    `json:"imageVersion"`
}

type Country struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	NameForURL      string `json:"nameForURL"`
	SportTypes      []int  `json:"sportTypes"`
	IsInternational bool   `json:"isInternational"`
	IsNotReal       bool   `json:"isNotReal"`
	ImageVersion    int    `json:"imageVersion"`
}

type Competition struct {
	ID               int    `json:"id"`
	CountryID        int    `json:"countryId"`
	SportID          int    `json:"sportId"`
	Name             string `json:"name"`
	ShortName        string `json:"shortName"`
	LongName         string `json:"longName"`
	NameForURL       string `json:"nameForURL"`
	PopularityRank   int64  `json:"popularityRank"`
	TotalGames       int    `json:"totalGames"`
	LiveGames        int    `json:"liveGames"`
	HasActiveGames   bool   `json:"hasActiveGames"`
	CurrentSeasonNum int    `json:"currentSeasonNum"`
	CurrentStageNum  int    `json:"currentStageNum"`
	CurrentPhaseName string `json:"currentPhaseName"`
	CompetitorsType  int    `json:"competitorsType"`
	IsInternational  bool   `json:"isInternational"`
	HideOnCatalog    bool   `json:"hideOnCatalog"`
	HideOnSearch     bool   `json:"hideOnSearch"`
	Color            string `json:"color"`
	ImageVersion     int    `json:"imageVersion"`
}

// Competitor is a team (or, for individual sports, a player). The
// allscores payload embeds the two participants directly on the Game
// rather than referencing a dictionary, so both shapes decode into
// this type.
type Competitor struct {
	ID                int     `json:"id"`
	CountryID         int     `json:"countryId"`
	SportID           int     `json:"sportId"`
	Name              string  `json:"name"`
	ShortName         string  `json:"shortName"`
	LongName          string  `json:"longName"`
	SymbolicName      string  `json:"symbolicName"`
	NameForURL        string  `json:"nameForURL"`
	Score             float64 `json:"score"`
	AggregatedScore   float64 `json:"aggregatedScore"`
	RedCards          int     `json:"redCards"`
	IsWinner          bool    `json:"isWinner"`
	IsQualified       bool    `json:"isQualified"`
	InPossession      bool    `json:"inPossession"`
	Type              int     `json:"type"`
	CompetitorNum     int     `json:"competitorNum"`
	MainCompetitionID int     `json:"mainCompetitionId"`
	PopularityRank    int64   `json:"popularityRank"`
	Color             string  `json:"color"`
	AwayColor         string  `json:"awayColor"`
	ImageVersion      int     `json:"imageVersion"`
}

type Game struct {
	ID                     int        `json:"id"`
	SportID                int        `json:"sportId"`
	CompetitionID          int        `json:"competitionId"`
	CompetitionDisplayName string     `json:"competitionDisplayName"`
	SeasonNum              int        `json:"seasonNum"`
	StageNum               int        `json:"stageNum"`
	GroupNum               int        `json:"groupNum"`
	RoundNum               int        `json:"roundNum"`
	RoundName              string     `json:"roundName"`
	GroupName              string     `json:"groupName"`
	StageName              string     `json:"stageName"`
	StartTime              string     `json:"startTime"`
	StatusGroup            int        `json:"statusGroup"`
	StatusText             string     `json:"statusText"`
	ShortStatusText        string     `json:"shortStatusText"`
	GameTime               float64    `json:"gameTime"`
	GameTimeDisplay        string     `json:"gameTimeDisplay"`
	JustEnded              bool       `json:"justEnded"`
	Winner                 int        `json:"winner"`
	HomeCompetitor         Competitor `json:"homeCompetitor"`
	AwayCompetitor         Competitor `json:"awayCompetitor"`
	IsHomeAwayInverted     bool       `json:"isHomeAwayInverted"`
	AggregateText          string     `json:"aggregateText"`
}

// StatusGroup values on the modern payload. The legacy /data/games
// feed exposed a fine-grained numeric GT code; the web API replaced
// it with this coarse grouping plus a *localized* StatusText, so
// telling "cancelled" apart from "finished" requires inspecting the
// text (see internal/scraper/scores365/feed.go).
const (
	StatusGroupScheduled = 2
	StatusGroupLive      = 3
	StatusGroupEnded     = 4
)

// ScoreNone is the sentinel 365scores uses for "no score yet" on
// scheduled fixtures (Competitor.Score == -1).
const ScoreNone = -1

// League is the normalized competition row the catalog discovery
// path consumes. It is produced from Competition rows returned by
// FetchCompetitions, not from any markup.
type League struct {
	Source         string
	SourceLeagueId string
	Name           string
	Sport          string
	Country        string
	NameForURL     string
}

func NewClient(opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.ImageBaseURL == "" {
		opts.ImageBaseURL = DefaultImageBaseURL
	}
	if opts.AppTypeID == 0 {
		opts.AppTypeID = DefaultAppTypeID
	}
	if opts.LangID == 0 {
		opts.LangID = DefaultLangID
	}
	if opts.TimezoneName == "" {
		opts.TimezoneName = DefaultTimezoneName
	}
	if opts.UserCountryID == 0 {
		opts.UserCountryID = DefaultUserCountryID
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = defaultCacheTTL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:       strings.TrimRight(opts.BaseURL, "/"),
		imageBaseURL:  strings.TrimRight(opts.ImageBaseURL, "/"),
		appTypeID:     opts.AppTypeID,
		langID:        opts.LangID,
		timezoneName:  opts.TimezoneName,
		userCountryID: opts.UserCountryID,
		sportIDs:      append([]int(nil), opts.SportIDs...),
		cacheTTL:      opts.CacheTTL,
		http:          opts.HTTPClient,
		referer:       Referer,
		log:           opts.Logger,
		cache:         make(map[string]cacheEntry),
	}
}

// SportIDs returns the configured sport filter used by
// FetchCompetitions when the caller does not pass one.
func (c *Client) SportIDs() []int { return append([]int(nil), c.sportIDs...) }

// TimezoneName returns the IANA zone the client asks the API to
// bucket fixtures by.
func (c *Client) TimezoneName() string { return c.timezoneName }

// commonParams returns the query parameters every /web/ endpoint
// expects.
func (c *Client) commonParams() url.Values {
	v := url.Values{}
	v.Set("appTypeId", strconv.Itoa(c.appTypeID))
	v.Set("langId", strconv.Itoa(c.langID))
	v.Set("timezoneName", c.timezoneName)
	v.Set("userCountryId", strconv.Itoa(c.userCountryID))
	return v
}

// FetchGames returns every fixture 365scores publishes for the given
// calendar day, in the client's configured timezone.
//
// The API takes a startDate/endDate pair in DD/MM/YYYY form. Passing
// a real multi-day range returns an empty payload, so we always ask
// for a single day and let callers iterate. Unlike the legacy
// /data/games feed this works for arbitrary past and future dates,
// which is what makes a multi-day scrape possible at all.
func (c *Client) FetchGames(ctx context.Context, date time.Time) (*Feed, error) {
	day := date.Format("02/01/2006")
	params := c.commonParams()
	params.Set("startDate", day)
	params.Set("endDate", day)
	params.Set("onlyMajorGames", "false")
	params.Set("withTop", "true")
	return c.fetchFeed(ctx, "/web/games/allscores/", params)
}

// FetchCompetitions returns the competition catalog for the given
// sports. Without the sports parameter the API only returns a short
// "popular competitions" list, so an empty sportIDs falls back to the
// client's configured SportIDs; if those are empty too the request
// goes out unfiltered and returns that short list.
func (c *Client) FetchCompetitions(ctx context.Context, sportIDs []int) (*Feed, error) {
	if len(sportIDs) == 0 {
		sportIDs = c.sportIDs
	}
	params := c.commonParams()
	if len(sportIDs) > 0 {
		params.Set("sports", joinInts(sportIDs))
	}
	return c.fetchFeed(ctx, "/web/competitions/", params)
}

// FetchSports returns the sport dictionary (id, localized name and
// the canonical nameForURL slug).
func (c *Client) FetchSports(ctx context.Context) ([]Sport, error) {
	feed, err := c.fetchFeed(ctx, "/web/sports/", c.commonParams())
	if err != nil {
		return nil, err
	}
	return feed.Sports, nil
}

// FetchCountries returns the country dictionary used to resolve
// Competition.CountryID into a human-readable region.
func (c *Client) FetchCountries(ctx context.Context) ([]Country, error) {
	feed, err := c.fetchFeed(ctx, "/web/countries/", c.commonParams())
	if err != nil {
		return nil, err
	}
	return feed.Countries, nil
}

func (c *Client) fetchFeed(ctx context.Context, path string, params url.Values) (*Feed, error) {
	endpoint := c.baseURL + path + "?" + params.Encode()

	c.cacheMu.Lock()
	if e, ok := c.cache[endpoint]; ok && time.Now().Before(e.expiresAt) {
		c.cacheMu.Unlock()
		return e.feed, nil
	}
	c.cacheMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("scores365: build request %s: %w", path, err)
	}
	req.Header.Set("Referer", c.referer)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	body, err := c.doWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	var feed Feed
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("scores365: decode %s: %w", path, err)
	}

	c.cacheMu.Lock()
	c.cache[endpoint] = cacheEntry{feed: &feed, expiresAt: time.Now().Add(c.cacheTTL)}
	c.cacheMu.Unlock()
	return &feed, nil
}

// doWithRetry performs a GET with exponential backoff on 429 and 5xx.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) ([]byte, error) {
	backoffs := []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	var lastErr error
	for _, d := range backoffs {
		if d > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		cloned := req.Clone(ctx)
		resp, err := c.http.Do(cloned)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("scores365: 429")
			if c.log != nil {
				c.log.Warn("scores365: rate-limited, backing off", "next_backoff", d.String())
			}
			continue
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("scores365: status %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("scores365: status %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	return nil, fmt.Errorf("scores365: exhausted retries: %w", lastErr)
}

// ImageURL builds a badge URL on the 365scores image CDN. The CDN is
// a Cloudinary deployment: the transformation segment asks for a PNG
// bounded to 68x68 with a per-collection default placeholder, and the
// /v<imageVersion>/ segment busts the cache whenever upstream
// replaces the asset.
//
// collection is the Cloudinary folder: "Competitors", "Competitions"
// or "Countries".
func (c *Client) ImageURL(collection string, id int, imageVersion int) string {
	return imageURL(c.imageBaseURL, collection, id, imageVersion)
}

// CompetitorImageURL returns the badge URL for a team or player.
func (c *Client) CompetitorImageURL(id, imageVersion int) string {
	return c.ImageURL(CollectionCompetitors, id, imageVersion)
}

// CompetitionImageURL returns the badge URL for a competition.
func (c *Client) CompetitionImageURL(id, imageVersion int) string {
	return c.ImageURL(CollectionCompetitions, id, imageVersion)
}

// CountryImageURL returns the flag URL for a country.
func (c *Client) CountryImageURL(id, imageVersion int) string {
	return c.ImageURL(CollectionCountries, id, imageVersion)
}

// Cloudinary folder names on imagecache.365scores.com.
const (
	CollectionCompetitors  = "Competitors"
	CollectionCompetitions = "Competitions"
	CollectionCountries    = "Countries"
)

// CompetitorImageURL builds a competitor badge URL against the
// default image CDN host. It exists so packages that only hold a
// team ID (the logo fallback chain in internal/events) can build the
// same URL without depending on a configured *Client.
func CompetitorImageURL(id int, imageVersion int) string {
	return imageURL(DefaultImageBaseURL, CollectionCompetitors, id, imageVersion)
}

func imageURL(base, collection string, id, imageVersion int) string {
	if id <= 0 {
		return ""
	}
	if imageVersion <= 0 {
		imageVersion = 1
	}
	return fmt.Sprintf(
		"%s/image/upload/f_png,w_68,h_68,c_limit,q_auto:eco,d_%s:default1.png/v%d/%s/%d",
		strings.TrimRight(base, "/"), collection, imageVersion, collection, id,
	)
}

func joinInts(ints []int) string {
	parts := make([]string, 0, len(ints))
	for _, n := range ints {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ",")
}
