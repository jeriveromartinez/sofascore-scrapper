//go:build integration

package scraper

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCookieTransport lets us return a controlled response from the
// cookie-fetch request. The scraper's loadCookies hits baseURL+"/es/"
// so we route based on path.
type fakeCookieTransport struct {
	status int
	calls  atomic.Int64
}

func (t *fakeCookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return &http.Response{
		StatusCode: t.status,
		Body:       http.NoBody,
		Header:     http.Header{},
		Request:    req,
	}, nil
}

// TestRefreshCookies_ReturnsErrorOnFailure pins the new contract:
// refreshCookies must return the error from loadCookies instead of
// silently swallowing it. Before the fix, the function body was
//
//   if err := c.loadCookies(ctx, ""); err != nil {
//       return  // <- error lost; operator sees nothing
//   }
//   c.cookieLoaded.Store(true)
//
// and the caller (doRequest) had no signal that the refresh had
// failed. The fix returns the error and the caller logs it
// structured.
func TestRefreshCookies_ReturnsErrorOnFailure(t *testing.T) {
	transport := &fakeCookieTransport{status: http.StatusForbidden}
	c := &Client{
		httpClient:       &http.Client{Transport: transport},
		baseURL:          "http://example.com",
		maxRetries:       3,
		responseMaxBytes: 1024,
		maxBackoff:       1 * time.Millisecond,
	}
	c.cookieLoaded.Store(true) // pre-mark to skip ensureCookies

	err := c.refreshCookies(context.Background())
	if err == nil {
		t.Fatal("expected error from refreshCookies when loadCookies returns 403, got nil")
	}
	if transport.calls.Load() == 0 {
		t.Fatal("loadCookies was not called")
	}
}

// TestRefreshCookies_DoesNotMarkLoadedOnFailure is the regression
// guard: after a failed refresh, the next call to ensureCookies must
// still try to load cookies (cookieLoaded must remain false). Before
// the fix, this contract held by accident because the Store(true)
// was after the early return. After the fix, the contract must
// still hold. This test pins it explicitly.
func TestRefreshCookies_DoesNotMarkLoadedOnFailure(t *testing.T) {
	transport := &fakeCookieTransport{status: http.StatusInternalServerError}
	c := &Client{
		httpClient:       &http.Client{Transport: transport},
		baseURL:          "http://example.com",
		maxRetries:       3,
		responseMaxBytes: 1024,
		maxBackoff:       1 * time.Millisecond,
	}

	_ = c.refreshCookies(context.Background())
	if c.cookieLoaded.Load() {
		t.Fatal("cookieLoaded must remain false after a failed refresh")
	}
}
