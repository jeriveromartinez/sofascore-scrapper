// Package scores365 implements an HTTP client for the public 365scores.com
// REST API used to discover and fetch multi-sport fixtures.
//
// 365scores exposes two surfaces:
//
//   - https://webws.365scores.com/data/games — the daily fixtures feed
//     for every sport on the site (JSON, ~600KB uncompressed). No auth.
//   - https://www.365scores.com/sitemaps — one XML sitemap per
//     language × sport (e.g. en_tennis.xml, es_baloncesto.xml). Each
//     <loc> entry is a league URL whose last path segment is the
//     numeric competition ID we use as our SourceLeagueId.
//
// The site gates requests behind a Referer check, so every outgoing
// request sets Referer: https://www.365scores.com/ and a desktop
// Chrome User-Agent. Feed responses are cached in memory for 60s per
// UTC date; on 429 we back off 1s, 2s, 4s, 8s, 16s before giving up.
package scores365

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	Referer   = "https://www.365scores.com/"
)

type Options struct {
	BaseURL    string // "https://webws.365scores.com"
	SitemapURL string // "https://www.365scores.com/sitemaps"
	HTTPClient *http.Client
	Logger     interface {
		Warn(msg string, args ...any)
	}
}

type Client struct {
	baseURL    string
	sitemapURL string
	http       *http.Client
	referer    string
	log        interface {
		Warn(msg string, args ...any)
	}

	cacheMu sync.Mutex
	games   map[string]cacheEntry // key = "games:2026-09-13"
}

type cacheEntry struct {
	feed      *Feed
	expiresAt time.Time
}

type Feed struct {
	LastUpdateID      int64            `json:"LastUpdateID"`
	RequestedUpdateID int64            `json:"RequestedUpdateID"`
	CurrentDate       string           `json:"CurrentDate"`
	CurrentTimeUTC    string           `json:"CurrentTimeUTC"`
	TTL               int              `json:"TTL"`
	ScrollIndex       int              `json:"ScrollIndex"`
	Summary           map[string]any   `json:"Summary"`
	Countries         []Country        `json:"Countries"`
	Bookmakers        []map[string]any `json:"Bookmakers"`
	Competitions      []Competition    `json:"Competitions"`
	Games             []Game           `json:"Games"`
}

type Country struct {
	ID   int    `json:"ID"`
	Name string `json:"Name"`
}

type Competition struct {
	ID     int    `json:"ID"`
	Name   string `json:"Name"`
	SName  string `json:"SName"`
	CID    int    `json:"CID"`
	Gender int    `json:"Gender"`
	Type   int    `json:"Type"`
}

type Game struct {
	ID         int       `json:"ID"`
	Comp       int       `json:"Comp"`
	SID        int       `json:"SID"`
	Stage      int       `json:"Stage"`
	STID       int       `json:"STID"`
	GT         int       `json:"GT"`
	Completion *float64  `json:"Completion"`
	STime      string    `json:"STime"`
	ETime      string    `json:"ETime"`
	Scrs       []float64 `json:"Scrs"`
	Comps      []Team    `json:"Comps"`
	Winner     int       `json:"Winner"`
	IsFinished bool      `json:"IsFinished"`
	WebUrl     string    `json:"WebUrl"`
}

type Team struct {
	ID           int    `json:"ID"`
	Name         string `json:"Name"`
	SName        string `json:"SName"`
	SymbolicName string `json:"SymbolicName"`
	CID          int    `json:"CID"`
	Color        string `json:"Color"`
	Color2       string `json:"Color2"`
}

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
		opts.BaseURL = "https://webws.365scores.com"
	}
	if opts.SitemapURL == "" {
		opts.SitemapURL = "https://www.365scores.com/sitemaps"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    opts.BaseURL,
		sitemapURL: opts.SitemapURL,
		http:       opts.HTTPClient,
		referer:    Referer,
		log:        opts.Logger,
		games:      make(map[string]cacheEntry),
	}
}

func (c *Client) FetchGames(ctx context.Context, date time.Time) (*Feed, error) {
	cacheKey := "games:" + date.UTC().Format("2006-01-02")
	c.cacheMu.Lock()
	if e, ok := c.games[cacheKey]; ok && time.Now().Before(e.expiresAt) {
		c.cacheMu.Unlock()
		return e.feed, nil
	}
	c.cacheMu.Unlock()

	url := c.baseURL + "/data/games?lang=en"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("scores365: build request: %w", err)
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
		return nil, fmt.Errorf("scores365: decode feed: %w", err)
	}

	c.cacheMu.Lock()
	c.games[cacheKey] = cacheEntry{feed: &feed, expiresAt: time.Now().Add(60 * time.Second)}
	c.cacheMu.Unlock()
	return &feed, nil
}

func (c *Client) FetchSitemap(ctx context.Context, lang, sportSlug string) ([]League, error) {
	url := fmt.Sprintf("%s/%s_%s.xml", c.sitemapURL, lang, sportSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("scores365: build sitemap request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", c.referer)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scores365: sitemap request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // sport has no sitemap
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scores365: sitemap status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("scores365: read sitemap: %w", err)
	}
	return parseSitemap(body, sportSlug)
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

// parseSitemap parses the sitemap XML body and returns the league rows
// extracted from URLs. Two URL shapes are recognised:
//   - /<sport>/league/<slug>-<compId>      (tennis, soccer, ...)
//   - /<sport>/<country>/<slug>-<compId>   (basketball/usa/nba-47, ...)
var (
	sitemapLeaguePattern   = regexp.MustCompile(`/([a-z-]+)/league/([a-z0-9-]+)-(\d+)$`)
	sitemapCountryPattern  = regexp.MustCompile(`/([a-z-]+)/([^/]+)/([a-z0-9-]+)-(\d+)$`)
)

func parseSitemap(body []byte, sportSlug string) ([]League, error) {
	type sitemapURL struct {
		Locs []string `xml:"loc"`
	}
	type sitemapDoc struct {
		XMLName xml.Name     `xml:"urlset"`
		URLs    []sitemapURL `xml:"url"`
		Locs    []string     `xml:"loc"` // fallback for non-conformant flat sitemaps
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("scores365: decode sitemap xml: %w", err)
	}
	out := make([]League, 0)
	seen := make(map[string]bool) // dedupe by SourceLeagueId
	add := func(loc string) {
		if m := sitemapLeaguePattern.FindStringSubmatch(loc); m != nil {
			urlSport, slug, compId := m[1], m[2], m[3]
			if urlSport != sportSlug {
				return
			}
			if seen[compId] {
				return
			}
			seen[compId] = true
			out = append(out, League{
				Source:         "scores365",
				SourceLeagueId: compId,
				Name:           humanizeSlug(slug),
				Sport:          sportSlug,
				Country:        "",
				NameForURL:     slug,
			})
			return
		}
		if m := sitemapCountryPattern.FindStringSubmatch(loc); m != nil {
			urlSport, country, slug, compId := m[1], m[2], m[3], m[4]
			if urlSport != sportSlug {
				return
			}
			if seen[compId] {
				return
			}
			seen[compId] = true
			out = append(out, League{
				Source:         "scores365",
				SourceLeagueId: compId,
				Name:           humanizeSlug(country) + " " + humanizeSlug(slug),
				Sport:          sportSlug,
				Country:        country, // raw URL slug, bounded to 8 chars on real 365scores sitemaps (usa, spain, uk, ...)
				NameForURL:     country + "/" + slug,
			})
		}
	}
	for _, u := range doc.URLs {
		for _, loc := range u.Locs {
			add(loc)
		}
	}
	for _, loc := range doc.Locs {
		add(loc)
	}
	return out, nil
}

func humanizeSlug(slug string) string {
	parts := strings.Split(slug, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, strings.ToUpper(p[:1])+p[1:])
	}
	return strings.Join(out, " ")
}
