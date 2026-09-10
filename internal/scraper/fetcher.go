package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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
	Close() error
}

// fetchJS is executed in the browser context. credentials:'include'
// is required so the browser's cf_clearance cookie is forwarded;
// headers from Go are merged in; the result is JSON-serialized so
// the Go side can read status, body, and response headers
// (notably Retry-After).
const fetchJS = `(async (url, headers) => {
	const init = { credentials: 'include', headers: {} };
	if (headers) {
		for (const k of Object.keys(headers)) {
			init.headers[k] = headers[k];
		}
	}
	const r = await fetch(url, init);
	const headerObj = {};
	r.headers.forEach((v, k) => { headerObj[k.toLowerCase()] = v; });
	const body = await r.text();
	return JSON.stringify({
		status: r.status,
		body: body,
		headers: headerObj,
	});
})`

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

func (f *rodPageFetcher) Fetch(ctx context.Context, url string, headers map[string]string) (fetchResult, error) {
	page, err := f.acquire(ctx)
	if err != nil {
		return fetchResult{}, err
	}
	defer f.release(page)

	obj, err := page.Context(ctx).Eval(fetchJS, url, headers)
	if err != nil {
		return fetchResult{}, fmt.Errorf("scraper: browser fetch: %w", err)
	}

	raw, err := obj.Value.MarshalJSON()
	if err != nil || len(raw) == 0 {
		raw = []byte(obj.Description)
	}
	var decoded struct {
		Status  int                 `json:"status"`
		Body    string              `json:"body"`
		Headers map[string][]string `json:"headers"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fetchResult{}, fmt.Errorf("scraper: decode browser response: %w (raw=%q)", err, raw)
	}
	return fetchResult{
		Status:  decoded.Status,
		Body:    []byte(decoded.Body),
		Headers: decoded.Headers,
	}, nil
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
