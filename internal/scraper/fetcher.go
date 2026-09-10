package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// fetchResult is the subset of HTTP response data Client.doRequest
// actually uses; headers are read only to parse Retry-After on 429/503.
type fetchResult struct {
	Status  int
	Body    []byte
	Headers map[string][]string
}

// pageFetcher abstracts a single HTTP-like request. Production is
// rodPageFetcher (real headless Chromium handles TLS fingerprinting,
// cookies, and Cloudflare challenges); tests wire a fake so retry /
// refresh / backoff can be exercised without booting a browser.
type pageFetcher interface {
	Fetch(ctx context.Context, url string, headers map[string]string) (fetchResult, error)
	Warmup(ctx context.Context) error
	Close() error
}

const readBodyJS = `() => {
	const text = document.body ? document.body.innerText : '';
	return text;
}`

type rodPageFetcher struct {
	browser *rod.Browser
	pool    chan *rod.Page
	mu      sync.Mutex
	closed  bool
}

func newRodPageFetcher(browser *rod.Browser, size int) (*rodPageFetcher, error) {
	if size < 1 {
		size = 1
	}
	pool := make(chan *rod.Page, size)
	for i := 0; i < size; i++ {
		page, err := browser.Page(proto.TargetCreateTarget{})
		if err != nil {
			for p := range pool {
				_ = p.Close()
			}
			return nil, fmt.Errorf("scraper: create browser page %d: %w", i, err)
		}
		pool <- page
	}
	return &rodPageFetcher{browser: browser, pool: pool}, nil
}

// Warmup navigates one pooled page to the homepage and waits for
// it to settle. The browser context now holds whatever session
// cookies Fastly issued, and other pages inherit them on
// subsequent navigations. Without this the edge sees a cold
// client hitting /api/v1/* directly and returns the
// 403-challenge JSON body.
func (f *rodPageFetcher) Warmup(ctx context.Context) error {
	page, err := f.acquire(ctx)
	if err != nil {
		return err
	}
	defer f.release(page)

	if err := page.Context(ctx).Navigate("https://www.sofascore.com/es/"); err != nil {
		return fmt.Errorf("scraper: warmup navigate: %w", err)
	}
	// WaitLoad blocks until the document is fully parsed and
	// sub-resources stop firing. Without it Navigate returns as
	// soon as the request is sent and readyState is still
	// "loading" — SPA bundles haven't executed yet, no SPA
	// globals are present, and Fastly's edge may not have
	// finished setting cookies.
	if err := page.Context(ctx).WaitLoad(); err != nil {
		// not fatal: we can still attempt subsequent fetches
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return nil
	}
}

func (f *rodPageFetcher) Fetch(ctx context.Context, url string, headers map[string]string) (fetchResult, error) {
	page, err := f.acquire(ctx)
	if err != nil {
		return fetchResult{}, err
	}
	defer f.release(page)

	// Top-level navigation, not fetch(): top-level navigations are
	// exempt from CORS, so the browser actually shows us the
	// response body. fetch() from inside page.Eval would be blocked
	// by the browser because SofaScore's edge does not return
	// Access-Control-Allow-Origin: *.
	if err := page.Context(ctx).Navigate(url); err != nil {
		return fetchResult{}, fmt.Errorf("scraper: browser navigate: %w", err)
	}
	// WaitLoad ensures the document (the JSON-as-HTML page) is
	// fully rendered before we read its body. Without this, the
	// page might still be parsing and innerText returns an
	// empty string.
	if err := page.Context(ctx).WaitLoad(); err != nil {
		// not fatal: we can still attempt to read whatever's there
	}

	obj, err := page.Context(ctx).Eval(readBodyJS)
	if err != nil {
		return fetchResult{}, fmt.Errorf("scraper: read body: %w", err)
	}

	body := []byte(obj.Value.String())
	if obj.Description != "" && len(body) == 0 {
		body = []byte(obj.Description)
	}

	status := http.StatusOK
	trimmed := bytes.TrimSpace(body)

	if bytes.HasPrefix(trimmed, []byte("{")) {
		// Fastly returns HTTP 200 with a JSON body that reports the
		// block, e.g. {"error":{"code":403,"reason":"challenge"}}.
		// Detect those and surface them as a non-2xx so doRequest
		// can apply its retry / refresh path.
		var probe struct {
			Error *struct {
				Code    int    `json:"code"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(trimmed, &probe); err == nil && probe.Error != nil && probe.Error.Code != 0 {
			status = probe.Error.Code
			if status < 400 {
				status = http.StatusForbidden
			}
		}
	} else if !bytes.HasPrefix(trimmed, []byte("[")) {
		// Non-JSON body (HTML challenge page, empty doc, etc.).
		status = http.StatusBadGateway
	}

	return fetchResult{Status: status, Body: body, Headers: nil}, nil
}

func (f *rodPageFetcher) acquire(ctx context.Context) (*rod.Page, error) {
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return nil, fmt.Errorf("scraper: fetcher closed")
		}
		f.mu.Unlock()
		select {
		case page := <-f.pool:
			return page, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (f *rodPageFetcher) release(page *rod.Page) {
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		_ = page.Close()
		return
	}
	select {
	case f.pool <- page:
	default:
		_ = page.Close()
	}
}

func (f *rodPageFetcher) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.mu.Unlock()

	for {
		select {
		case page := <-f.pool:
			_ = page.Close()
		default:
			err := f.browser.Close()
			return err
		}
	}
}
