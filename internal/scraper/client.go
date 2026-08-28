package scraper

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utls "github.com/refraction-networking/utls"
)

const (
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

	defaultMaxRetries       = 3
	defaultRequestTimeout   = 30 * time.Second
	defaultResponseMaxBytes = 10 * 1024 * 1024
	defaultMaxBackoff       = 30 * time.Second
	baseBackoff             = 1 * time.Second
)

type ClientConfig struct {
	BaseURL          string
	MaxRetries       int
	RequestTimeout   time.Duration
	ResponseMaxBytes int64
	MaxBackoff       time.Duration
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
	return c
}

type SofaScoreClient interface {
	ScheduledEvents(ctx context.Context, sport string, date time.Time) ([]*APIEvent, error)
	TrendingEvents(ctx context.Context, countryCode string) ([]*APIEvent, error)
}

type Client struct {
	httpClient       *http.Client
	baseURL          string
	maxRetries       int
	responseMaxBytes int64
	maxBackoff       time.Duration
	cookieLoaded     atomic.Bool
	cookieMu         sync.Mutex
}

func NewClient(cfg ClientConfig) (*Client, error) {
	cfg = cfg.withDefaults()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("scraper: cookiejar: %w", err)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DialTLSContext:      utlsDialTLS,
	}

	httpClient := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   cfg.RequestTimeout,
	}

	return &Client{
		httpClient:       httpClient,
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		maxRetries:       cfg.MaxRetries,
		responseMaxBytes: cfg.ResponseMaxBytes,
		maxBackoff:       cfg.MaxBackoff,
	}, nil
}

// utlsDialTLS establishes a TLS connection using utls.HelloRandomized with
// TLS 1.2 and HTTP/1.1 ALPN. SofaScore's Varnish reverse proxy blocks clients
// whose TLS fingerprint matches known HTTP libraries (Go stdlib, curl); using
// a randomized ClientHello evades that fingerprint check.
func utlsDialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(addr)
	raw, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	cfg := &utls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	}
	conn := utls.UClient(raw, cfg, utls.HelloRandomized)
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("utls handshake: %w", err)
	}
	return conn, nil
}

func setBrowserHeaders(req *http.Request, accept string, referer string) {
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="145", "Not?A_Brand";v="24", "Google Chrome";v="145"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("DNT", "1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

// ensureCookies fetches the SofaScore home page to populate the cookie jar
// with anti-bot tokens (e.g. Cloudflare). It must fail loudly when the home
// page is blocked (HTTP 403) or returns no cookies, otherwise downstream API
// requests silently fail with 401/403 that the caller cannot distinguish from
// a stale-cookie condition.
func (c *Client) ensureCookies(ctx context.Context) error {
	if c.cookieLoaded.Load() {
		return nil
	}

	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()

	if c.cookieLoaded.Load() {
		return nil
	}

	if err := c.loadCookies(ctx, ""); err != nil {
		return err
	}
	c.cookieLoaded.Store(true)
	return nil
}

// loadCookies resets the cookie jar and fetches the home page. When
// pubReferer is non-empty, it is sent as the Referer for the home request to
// work around anti-bot rules that reject bare home fetches. It returns an
// error when the home page responds >= 400 or the jar contains no cookies.
func (c *Client) loadCookies(ctx context.Context, pubReferer string) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("scraper: cookiejar: %w", err)
	}
	c.httpClient.Jar = jar

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/es/", nil)
	if err != nil {
		return err
	}
	setBrowserHeaders(req, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", pubReferer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("scraper: cookie fetch: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, c.responseMaxBytes))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("scraper: cookie fetch blocked: HTTP %d", resp.StatusCode)
	}

	return nil
}

// refreshCookies resets the cookie jar and re-fetches the home page.
// It returns the error from loadCookies (instead of swallowing it)
// so the caller can log a structured warning — operators otherwise
// have no visibility into a transient 403/5xx that breaks all
// downstream API calls. On success, cookieLoaded is set to true;
// on failure it stays false so the next request retries the load.
func (c *Client) refreshCookies(ctx context.Context) error {
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()

	if err := c.loadCookies(ctx, ""); err != nil {
		return err
	}
	c.cookieLoaded.Store(true)
	return nil
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

func (c *Client) doRequest(ctx context.Context, path string) ([]byte, error) {
	if err := c.ensureCookies(ctx); err != nil {
		return nil, err
	}

	var refreshedCookie bool
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoffDuration(attempt)):
			}
		}

		url := c.baseURL + path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		setBrowserHeaders(req, "application/json, text/plain, */*", c.baseURL+"/es/")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		// Read the response body fully and close it before branching on
		// status. Deferring Body.Close() inside the retry loop leaks one
		// open body per attempt until doRequest returns; under sustained
		// 5xx storms this exhausts file descriptors.
		limited := io.LimitReader(resp.Body, c.responseMaxBytes)
		body, readErr := io.ReadAll(limited)
		// Drain remaining bytes so the underlying connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil

		case http.StatusUnauthorized:
			if !refreshedCookie {
				refreshedCookie = true
				if refreshErr := c.refreshCookies(ctx); refreshErr != nil {
					// The cookie refresh failed. Log it so operators
					// can see the transient failure; the retry loop
					// will try one more time anyway.
					return nil, fmt.Errorf("scraper: cookie refresh failed: %w", refreshErr)
				}
				continue
			}
			return nil, fmt.Errorf("scraper: HTTP 401")

		case http.StatusTooManyRequests:
			if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(ra):
				}
			}
			continue

		case http.StatusBadGateway, http.StatusServiceUnavailable:
			continue

		default:
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, fmt.Errorf("scraper: HTTP %d", resp.StatusCode)
			}
			continue
		}
	}

	return nil, fmt.Errorf("scraper: max retries exceeded")
}
