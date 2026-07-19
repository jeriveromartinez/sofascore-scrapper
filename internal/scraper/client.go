package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

func setBrowserHeaders(req *http.Request, accept string, referer string) {
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func (c *Client) ensureCookies(ctx context.Context) error {
	if c.cookieLoaded.Load() {
		return nil
	}

	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()

	if c.cookieLoaded.Load() {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/es/", nil)
	if err != nil {
		return err
	}
	setBrowserHeaders(req, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("scraper: cookie fetch: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, c.responseMaxBytes))

	c.cookieLoaded.Store(true)
	return nil
}

func (c *Client) refreshCookies(ctx context.Context) {
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()

	jar, _ := cookiejar.New(nil)
	c.httpClient.Jar = jar
	c.cookieLoaded.Store(false)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/es/", nil)
	if err != nil {
		return
	}
	setBrowserHeaders(req, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", "")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, c.responseMaxBytes))
	c.cookieLoaded.Store(true)
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
	path := fmt.Sprintf("/api/v1/sport/%s/scheduled-events/%s", sport, date.Format("2006-01-02"))
	body, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	var list EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("scraper: parse scheduled events: %w", err)
	}
	return list.Events, nil
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
		defer resp.Body.Close()

		limited := io.LimitReader(resp.Body, c.responseMaxBytes)
		body, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil

		case http.StatusUnauthorized:
			if !refreshedCookie {
				refreshedCookie = true
				c.refreshCookies(ctx)
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
