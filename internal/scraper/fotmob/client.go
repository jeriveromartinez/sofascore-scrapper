package fotmob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// PR #124: Chrome/120 is the user-agent the bjrsti gem uses
	// and the UA under which the FotMob /api/data/matches endpoint
	// was verified to return HTTP 200 without an x-mas token. The
	// previous Chrome/145 string was tied to a different endpoint
	// shape that no longer exists.
	browserUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	defaultMaxRetries     = 3
	defaultRequestTimeout = 30 * time.Second
	defaultMaxResponse    = 10 * 1024 * 1024
	defaultMaxBackoff     = 30 * time.Second
	baseBackoff           = 1 * time.Second
	rateLimit             = 200 * time.Millisecond
	// defaultTimezone matches bjrsti and the env var the operator
	// can override (FOTMOB_TIMEZONE). Europe/Paris keeps the daily
	// payload grouping aligned with FotMob's editorial day.
	defaultTimezone = "Europe/Paris"
)

type ClientConfig struct {
	BaseURL          string
	MaxRetries       int
	RequestTimeout   time.Duration
	ResponseMaxBytes int64
	MaxBackoff       time.Duration
	// Timezone is appended to the /api/data/matches URL as
	// `?timezone=…` and is what FotMob uses to bucket matches into
	// "today" / "yesterday" / "tomorrow". Default: Europe/Paris.
	Timezone string
}

func (c ClientConfig) withDefaults() ClientConfig {
	if c.BaseURL == "" {
		c.BaseURL = "https://www.fotmob.com"
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.ResponseMaxBytes <= 0 {
		c.ResponseMaxBytes = defaultMaxResponse
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = defaultMaxBackoff
	}
	if c.Timezone == "" {
		c.Timezone = defaultTimezone
	}
	return c
}

type Client struct {
	httpClient       *http.Client
	baseURL          string
	maxRetries       int
	responseMaxBytes int64
	maxBackoff       time.Duration
	timezone         string
	lastCall         time.Time
	rateMu           chan struct{}
}

func NewClient(cfg ClientConfig) *Client {
	cfg = cfg.withDefaults()
	return &Client{
		httpClient:       &http.Client{Timeout: cfg.RequestTimeout},
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		maxRetries:       cfg.MaxRetries,
		responseMaxBytes: cfg.ResponseMaxBytes,
		maxBackoff:       cfg.MaxBackoff,
		timezone:         cfg.Timezone,
		rateMu:           make(chan struct{}, 1),
	}
}

// ScheduledEvents fetches the full day for `date` from FotMob's
// /api/data/matches endpoint and returns only the matches that
// belong to the league identified by `leagueID` (FotMob's id is a
// numeric string in the catalog; the URL itself does not carry a
// league filter). An unknown league is a valid empty result — the
// scheduler treats an empty day as a no-op and the caller does not
// have to special-case it.
func (c *Client) ScheduledEvents(ctx context.Context, leagueID string, date time.Time) ([]apiMatch, error) {
	dayMatches, err := c.dayMatches(ctx, date)
	if err != nil {
		return nil, err
	}
	want, err := strconv.ParseInt(leagueID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("fotmob: invalid league id %q: %w", leagueID, err)
	}
	for _, lg := range dayMatches.Leagues {
		if lg.Id == want {
			if lg.Matches == nil {
				return []apiMatch{}, nil
			}
			return lg.Matches, nil
		}
	}
	return []apiMatch{}, nil
}

// dayMatches performs the actual HTTP round-trip and returns the
// full per-league payload for the day. The path format is the
// documented one (`/api/data/matches?date=YYYYMMDD&timezone=…`);
// the date format is YYYYMMDD with no hyphens to match bjrsti and
// the upstream convention.
func (c *Client) dayMatches(ctx context.Context, date time.Time) (apiMatchesResponse, error) {
	path := fmt.Sprintf("/api/data/matches?date=%s&timezone=%s",
		date.UTC().Format("20060102"),
		c.timezone,
	)
	body, err := c.doRequest(ctx, path)
	if err != nil {
		return apiMatchesResponse{}, err
	}
	var resp apiMatchesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return apiMatchesResponse{}, fmt.Errorf("fotmob: parse matches: %w", err)
	}
	return resp, nil
}

// Suggest queries FotMob's public suggest endpoint and returns the raw
// JSON envelope. The shape is loosely documented: an object whose
// `suggestions` array carries typed entries (league/team/player). The
// caller (Source.SearchLeagues) is responsible for filtering by type
// and mapping to the domain shape.
func (c *Client) Suggest(ctx context.Context, term string) (apiSuggestResponse, error) {
	path := fmt.Sprintf("/api/searchapi/suggest?term=%s&lang=en", url.QueryEscape(term))
	body, err := c.doRequest(ctx, path)
	if err != nil {
		return apiSuggestResponse{}, err
	}
	var resp apiSuggestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return apiSuggestResponse{}, fmt.Errorf("fotmob: parse suggest: %w", err)
	}
	return resp, nil
}

func (c *Client) doRequest(ctx context.Context, path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt)):
			}
		}

		if err := c.waitRateLimit(ctx); err != nil {
			return nil, err
		}

		url := c.baseURL + path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		setBrowserHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		limited := io.LimitReader(resp.Body, c.responseMaxBytes)
		body, _ := io.ReadAll(limited)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil
		case http.StatusTooManyRequests:
			if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(ra):
				}
			}
			lastErr = fmt.Errorf("fotmob: 429")
			continue
		case http.StatusBadGateway, http.StatusServiceUnavailable:
			lastErr = fmt.Errorf("fotmob: %d", resp.StatusCode)
			continue
		default:
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, fmt.Errorf("fotmob: HTTP %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("fotmob: %d", resp.StatusCode)
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("max retries exceeded")
	}
	return nil, lastErr
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	select {
	case c.rateMu <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.rateMu }()
	if !c.lastCall.IsZero() {
		elapsed := time.Since(c.lastCall)
		if elapsed < rateLimit {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rateLimit - elapsed):
			}
		}
	}
	c.lastCall = time.Now()
	return nil
}

func (c *Client) backoff(attempt int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempt-1))) * baseBackoff
	if d > c.maxBackoff {
		d = c.maxBackoff
	}
	return d
}

func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	if sec, err := strconv.Atoi(s); err == nil {
		return time.Duration(sec) * time.Second
	}
	t, err := http.ParseTime(s)
	if err != nil {
		return 0
	}
	if d := time.Until(t); d > 0 {
		return d
	}
	return 0
}

func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "es-419,es;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	// PR #124: Sec-Ch-Ua now advertises Chrome/120 instead of
	// Chrome/145 so the client presents a consistent browser
	// fingerprint with the User-Agent header.
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="120", "Not?A_Brand";v="24", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("DNT", "1")
	req.Header.Set("Referer", "https://www.fotmob.com/es-419")
}
