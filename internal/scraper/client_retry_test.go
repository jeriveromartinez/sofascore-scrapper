package scraper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// alwaysFail503Transport returns 503 Service Unavailable for every request so
// the retry loop in doRequest runs to exhaustion.
type alwaysFail503Transport struct {
	calls atomic.Int64
}

func (t *alwaysFail503Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Status:     "503 Service Unavailable",
		Body:       io.NopCloser(strings.NewReader("service unavailable")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// leakTrackingTransport wraps a base RoundTripper and tracks open response
// bodies so the test can detect leaks across retry boundaries.
type leakTrackingTransport struct {
	base    http.RoundTripper
	open    atomic.Int32
	maxOpen atomic.Int32
}

func (t *leakTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	cur := t.open.Add(1)
	for {
		max := t.maxOpen.Load()
		if cur <= max {
			break
		}
		if t.maxOpen.CompareAndSwap(max, cur) {
			break
		}
	}
	resp.Body = &trackingReadCloser{ReadCloser: resp.Body, open: &t.open}
	return resp, nil
}

type trackingReadCloser struct {
	io.ReadCloser
	open *atomic.Int32
}

func (b *trackingReadCloser) Close() error {
	b.open.Add(-1)
	return b.ReadCloser.Close()
}

// TestDoRequest_ClosesBodyBetweenRetries verifies that doRequest closes the
// previous response body before issuing the next retry. The current
// implementation defers resp.Body.Close() inside the retry loop, which
// accumulates one open body per attempt and only releases them when the
// function returns. Under sustained 503/502 storms this exhausts file
// descriptors on the host.
func TestDoRequest_ClosesBodyBetweenRetries(t *testing.T) {
	base := &alwaysFail503Transport{}
	transport := &leakTrackingTransport{base: base}

	c := &Client{
		httpClient:       &http.Client{Transport: transport},
		baseURL:          "http://example.com",
		maxRetries:       3,
		responseMaxBytes: 1024,
		maxBackoff:       1 * time.Millisecond,
	}
	// ensureCookies() would otherwise hit the same transport; pre-mark
	// cookies as loaded so the test exercises the retry loop only.
	c.cookieLoaded.Store(true)

	_, err := c.doRequest(context.Background(), "/retry-503")
	if err == nil {
		t.Fatal("expected error from exhausted retries, got nil")
	}

	if got := base.calls.Load(); got != int64(c.maxRetries+1) {
		t.Errorf("attempts = %d, want %d", got, c.maxRetries+1)
	}
	if got := transport.maxOpen.Load(); got > 1 {
		t.Errorf("response body leak: max %d bodies open simultaneously during retry loop, want <= 1", got)
	}
	if got := transport.open.Load(); got != 0 {
		t.Errorf("response bodies still open after doRequest returned: %d", got)
	}
}

// TestDoRequest_ClosesBodyOnSuccess verifies that on the successful retry
// the previously-leaked 503 bodies are released before the function returns
// the 200 body. Without the fix, all 4 bodies stay open until return.
func TestDoRequest_ClosesBodyOnSuccess(t *testing.T) {
	var calls atomic.Int64
	base := &flakyTransport{calls: &calls, failUntil: 2}
	transport := &leakTrackingTransport{base: base}

	c := &Client{
		httpClient:       &http.Client{Transport: transport},
		baseURL:          "http://example.com",
		maxRetries:       3,
		responseMaxBytes: 1024,
		maxBackoff:       1 * time.Millisecond,
	}
	c.cookieLoaded.Store(true)

	body, err := c.doRequest(context.Background(), "/flaky")
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q, want %q", body, `{"ok":true}`)
	}
	if got := transport.maxOpen.Load(); got > 1 {
		t.Errorf("response body leak: max %d bodies open simultaneously, want <= 1", got)
	}
	if got := transport.open.Load(); got != 0 {
		t.Errorf("response bodies still open after doRequest returned: %d", got)
	}
}

// flakyTransport returns 503 for the first failUntil requests and 200 OK
// thereafter.
type flakyTransport struct {
	calls     *atomic.Int64
	failUntil int64
}

func (t *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	n := t.calls.Add(1)
	if n <= t.failUntil {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "503 Service Unavailable",
			Body:       io.NopCloser(strings.NewReader("service unavailable")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}
