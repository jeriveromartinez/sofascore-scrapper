//go:build rod_integration

package scraper

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestClient_RodFetchesRealSofascore exercises the full production
// path: NewClient launches a real headless Chromium (downloading it
// on first run via rod's launcher), the page pool issues fetches
// through page.Eval, and we hit the live SofaScore API.
//
// Run with:
//
//	go test -tags rod_integration -run TestClient_RodFetchesRealSofascore ./internal/scraper/...
//
// Set SOFASCORE_INTEGRATION=1 to opt in; otherwise the test skips.
// Without this opt-in, "go test ./..." stays browser-free.
func TestClient_RodFetchesRealSofascore(t *testing.T) {
	if os.Getenv("SOFASCORE_INTEGRATION") == "" {
		t.Skip("set SOFASCORE_INTEGRATION=1 to run the rod-backed integration test (downloads Chromium)")
	}

	c, err := NewClient(ClientConfig{RequestTimeout: 60 * time.Second, MaxBackoff: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewClient (rod): %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	events, err := c.TrendingEvents(ctx, "MX")
	if err != nil {
		t.Fatalf("TrendingEvents via rod: %v", err)
	}
	t.Logf("got %d trending events", len(events))
}
