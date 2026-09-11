package fotmob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	browserUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	defaultMaxRetries     = 3
	defaultRequestTimeout = 30 * time.Second
	defaultMaxResponse    = 10 * 1024 * 1024
	defaultMaxBackoff     = 30 * time.Second
	baseBackoff           = 1 * time.Second
	rateLimit             = 200 * time.Millisecond
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
	return c
}

type Client struct {
	httpClient       *http.Client
	baseURL          string
	maxRetries       int
	responseMaxBytes int64
	maxBackoff       time.Duration
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
		rateMu:           make(chan struct{}, 1),
	}
}

func (c *Client) ScheduledEvents(ctx context.Context, leagueID, date string) ([]apiMatch, error) {
	path := fmt.Sprintf("/api/leagues?id=%s&date=%s", leagueID, date)
	body, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	var resp apiMatchesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("fotmob: parse matches: %w", err)
	}
	return resp.Matches.AllMatches, nil
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
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="145", "Not?A_Brand";v="24", "Google Chrome";v="145"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("DNT", "1")
	req.Header.Set("Referer", "https://www.fotmob.com/es-419")
}
