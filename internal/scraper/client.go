package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRetries       = 3
	defaultRequestTimeout   = 30 * time.Second
	defaultResponseMaxBytes = 10 * 1024 * 1024
	defaultMaxBackoff       = 30 * time.Second
	baseBackoff             = 1 * time.Second
	defaultPagePoolSize     = 4
)

// ClientConfig is the tunables for NewClient. PagePoolSize is honored
// when constructing the rod-backed fetcher from config; tests that
// build a Client directly with their own pageFetcher may ignore it.
type ClientConfig struct {
	BaseURL          string
	MaxRetries       int
	RequestTimeout   time.Duration
	ResponseMaxBytes int64
	MaxBackoff       time.Duration
	PagePoolSize     int
}

func (c ClientConfig) withDefaults() ClientConfig {
	if c.BaseURL == "" {
		c.BaseURL = "https://www.sofascore.com"
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.ResponseMaxBytes <= 0 {
		c.ResponseMaxBytes = defaultResponseMaxBytes
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = defaultMaxBackoff
	}
	if c.PagePoolSize <= 0 {
		c.PagePoolSize = defaultPagePoolSize
	}
	return c
}

// SofaScoreClient is the interface Service depends on. NewClient
// returns a *Client that implements it.
type SofaScoreClient interface {
	ScheduledEvents(ctx context.Context, sport string, date time.Time) ([]*APIEvent, error)
	TrendingEvents(ctx context.Context, countryCode string) ([]*APIEvent, error)
}

// Client scrapes SofaScore via a real headless Chromium browser
// driven by rod. The browser handles cookies, TLS fingerprinting,
// and Cloudflare challenges transparently; doRequest only worries
// about status codes, backoff, and response size limits.
type Client struct {
	fetcher          pageFetcher
	baseURL          string
	maxRetries       int
	requestTimeout   time.Duration
	responseMaxBytes int64
	maxBackoff       time.Duration
	logger           *slog.Logger
}

// NewClient launches a headless Chromium (downloading it on first
// run via rod's launcher) and constructs a Client backed by it. The
// caller must invoke Close when done to release the browser
// subprocess. A non-nil error means no browser was launched and
// nothing needs to be closed.
func NewClient(cfg ClientConfig) (*Client, error) {
	cfg = cfg.withDefaults()

	browser, err := launchBrowser()
	if err != nil {
		return nil, err
	}
	fetcher, err := newRodPageFetcher(browser, cfg.PagePoolSize)
	if err != nil {
		_ = browser.Close()
		return nil, err
	}

	c := newClientWithFetcher(cfg, fetcher)

	// Warmup once before returning. Fastly's edge issues per-session
	// cookies after the first HTML navigation; subsequent API calls
	// from any page in the same browser context inherit them. The
	// warmup itself runs with the configured request timeout so a
	// stuck browser fails fast instead of hanging app startup.
	warmupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.fetcher.Warmup(warmupCtx); err != nil {
		c.logger.Warn("scraper: warmup failed", slog.String("error", err.Error()))
	}
	return c, nil
}

// newClientWithFetcher is the seam tests use to build a Client
// without booting a real browser. Production callers use NewClient.
func newClientWithFetcher(cfg ClientConfig, fetcher pageFetcher) *Client {
	cfg = cfg.withDefaults()
	return &Client{
		fetcher:          fetcher,
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		maxRetries:       cfg.MaxRetries,
		requestTimeout:   cfg.RequestTimeout,
		responseMaxBytes: cfg.ResponseMaxBytes,
		maxBackoff:       cfg.MaxBackoff,
		logger:           slog.Default(),
	}
}

// SetLogger overrides the default slog logger used by Client. Useful
// in tests that want to capture log output.
func (c *Client) SetLogger(l *slog.Logger) {
	if l != nil {
		c.logger = l
	}
}

// Close shuts down the underlying browser. Safe to call once;
// subsequent calls are no-ops.
func (c *Client) Close() error {
	return c.fetcher.Close()
}

func (c *Client) ScheduledEvents(ctx context.Context, sport string, date time.Time) ([]*APIEvent, error) {
	dateStr := date.Format("2006-01-02")
	tournamentsPath := fmt.Sprintf("/api/v1/sport/%s/scheduled-tournaments/%s/page/1", sport, dateStr)
	body, err := c.doRequest(ctx, tournamentsPath)
	if err != nil {
		return nil, err
	}
	var tournamentsResp ScheduledTournamentsResponse
	if err := json.Unmarshal(body, &tournamentsResp); err != nil {
		return nil, fmt.Errorf("scraper: parse scheduled tournaments: %w", err)
	}

	var allEvents []*APIEvent
	for _, t := range tournamentsResp.Scheduled {
		uniqueTournamentID := t.Tournament.UniqueTournament.ID
		if uniqueTournamentID == 0 {
			continue
		}
		eventsPath := fmt.Sprintf("/api/v1/unique-tournament/%d/scheduled-events/%s", uniqueTournamentID, dateStr)
		eventsBody, err := c.doRequest(ctx, eventsPath)
		if err != nil {
			continue
		}
		var list EventsListResponse
		if err := json.Unmarshal(eventsBody, &list); err != nil {
			continue
		}
		allEvents = append(allEvents, list.Events...)
	}
	return allEvents, nil
}

func (c *Client) TrendingEvents(ctx context.Context, countryCode string) ([]*APIEvent, error) {
	path := fmt.Sprintf("/api/v1/trending/events/%s/all", strings.ToUpper(countryCode))
	body, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	var list EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("scraper: parse trending events: %w", err)
	}
	return list.Events, nil
}

func (c *Client) requestHeaders(path string) map[string]string {
	return map[string]string{
		"Accept":             "application/json, text/plain, */*",
		"Accept-Language":    "es-ES,es;q=0.9,en-US;q=0.8,en;q=0.7",
		"Cache-Control":      "no-cache",
		"Pragma":             "no-cache",
		"Referer":            c.baseURL + "/es/",
		"Sec-Ch-Ua":          `"Chromium";v="145", "Not?A_Brand";v="24", "Google Chrome";v="145"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-origin",
	}
}

func (c *Client) backoffDuration(attempt int) time.Duration {
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

func bodyPreview(body []byte) []byte {
	const max = 200
	if len(body) <= max {
		return body
	}
	out := make([]byte, max+3)
	copy(out, body[:max])
	copy(out[max:], []byte("..."))
	return out
}

// doRequest performs one API call against the given baseURL-relative
// path with full retry / backoff / 401-refresh semantics. The actual
// transport lives in c.fetcher (rod-backed in production); doRequest
// stays transport-agnostic so retry policy is testable.
//
// On 401/403 the first occurrence triggers a brief "soft refresh"
// delay (the rod browser context re-issues cookies / handles
// challenges on the next request automatically) and then retries;
// further 401/403 responses surface as a hard error.
func (c *Client) doRequest(ctx context.Context, path string) ([]byte, error) {
	if c.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.requestTimeout)
		defer cancel()
	}

	var refreshedCookie bool
	url := c.baseURL + path
	headers := c.requestHeaders(path)

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoffDuration(attempt)):
			}
		}

		result, err := c.fetcher.Fetch(ctx, url, headers)
		if err != nil {
			c.logger.WarnContext(ctx, "scraper fetch error",
				slog.String("path", path),
				slog.Int("attempt", attempt),
				slog.String("error", err.Error()),
			)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		c.logger.DebugContext(ctx, "scraper fetch result",
			slog.String("path", path),
			slog.Int("attempt", attempt),
			slog.Int("status", result.Status),
			slog.Int("body_bytes", len(result.Body)),
			slog.String("body_preview", string(bodyPreview(result.Body))),
		)

		if int64(len(result.Body)) > c.responseMaxBytes {
			// rod hands the body to us already fully read; the
			// net/http-era io.LimitReader is now a post-read cap.
			return nil, fmt.Errorf("scraper: response exceeded %d bytes", c.responseMaxBytes)
		}

		switch result.Status {
		case http.StatusOK:
			return result.Body, nil

		case http.StatusUnauthorized, http.StatusForbidden:
			// Surface the Fastly/WAF response body so operators can see
			// why a request was blocked. Bodies are usually short
			// (challenge markup, error JSON, or empty).
			bodyPreview := string(result.Body)
			if len(bodyPreview) > 200 {
				bodyPreview = bodyPreview[:200] + "..."
			}
			if bodyPreview != "" {
				c.logger.WarnContext(ctx, "scraper blocked response",
					slog.Int("status", result.Status),
					slog.String("body", bodyPreview),
					slog.String("path", path),
				)
			}
			if !refreshedCookie {
				refreshedCookie = true
				if refreshErr := c.refreshCookies(ctx); refreshErr != nil {
					return nil, fmt.Errorf("scraper: cookie refresh failed: %w", refreshErr)
				}
				continue
			}
			return nil, fmt.Errorf("scraper: HTTP %d", result.Status)

		case http.StatusTooManyRequests:
			if v := result.Headers["Retry-After"]; len(v) > 0 {
				if ra := parseRetryAfter(v[0]); ra > 0 {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(ra):
					}
				}
			}
			continue

		case http.StatusBadGateway, http.StatusServiceUnavailable:
			continue

		default:
			if result.Status >= 400 && result.Status < 500 {
				return nil, fmt.Errorf("scraper: HTTP %d", result.Status)
			}
			continue
		}
	}

	return nil, fmt.Errorf("scraper: max retries exceeded")
}

// refreshCookies is the rod-era equivalent of the old loadCookies
// re-fetch. With a real browser the cf_clearance cookie lives in
// the browser context; the next Fetch will send whatever cookies
// the browser currently holds. A short sleep is enough to let the
// browser settle if it was mid-challenge.
func (c *Client) refreshCookies(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		return nil
	}
}
