package scraper

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// fakeFetcher is the test seam for pageFetcher. It lets a test pin
// the response per URL path (or per call count) and inspect how
// many times Fetch was invoked. It also tracks open responses so
// retry-loop tests can assert no resources leak across attempts.
type fakeFetcher struct {
	mu sync.Mutex

	// routes maps a URL substring -> handler. First match wins.
	routes map[string]func(ctx context.Context, call int) (fetchResult, error)
	// defaultHandler is used when no route matches.
	defaultHandler func(ctx context.Context, url string, call int) (fetchResult, error)

	calls atomic.Int64
	open  atomic.Int32

	closed atomic.Bool
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{routes: map[string]func(context.Context, int) (fetchResult, error){}}
}

func (f *fakeFetcher) route(substr string, h func(ctx context.Context, call int) (fetchResult, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[substr] = h
}

func (f *fakeFetcher) setDefault(h func(ctx context.Context, url string, call int) (fetchResult, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defaultHandler = h
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string, _ map[string]string) (fetchResult, error) {
	if f.closed.Load() {
		return fetchResult{}, errFetcherClosed
	}
	f.open.Add(1)
	defer f.open.Add(-1)

	call := f.calls.Add(1)

	f.mu.Lock()
	for substr, h := range f.routes {
		if strings.Contains(url, substr) {
			f.mu.Unlock()
			return h(ctx, int(call))
		}
	}
	h := f.defaultHandler
	f.mu.Unlock()
	if h == nil {
		return fetchResult{Status: http.StatusOK, Body: []byte(`{"events":[]}`)}, nil
	}
	return h(ctx, url, int(call))
}

func (f *fakeFetcher) Close() error {
	f.closed.Store(true)
	return nil
}

func (f *fakeFetcher) callCount() int64 { return f.calls.Load() }
func (f *fakeFetcher) openCount() int32 { return f.open.Load() }

var errFetcherClosed = &fetcherClosedError{}

type fetcherClosedError struct{}

func (*fetcherClosedError) Error() string { return "scraper: fetcher closed" }
