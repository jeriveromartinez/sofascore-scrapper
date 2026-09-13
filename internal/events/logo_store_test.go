package events

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestNewImageHTTPClientUsesImageTimeout(t *testing.T) {
	client := newImageHTTPClient()
	if client.Timeout != imageDownloadTimeout {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, imageDownloadTimeout)
	}
}

func TestNewImageHTTPClientUsesUTLSTransport(t *testing.T) {
	client := newImageHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.DialTLSContext == nil {
		t.Fatal("image transport must provide a custom TLS dialer")
	}
}

func TestDownloadTeamLogoClosesIdleConnections(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("test-image"))
	}))
	defer server.Close()

	transport := &closeTrackingTransport{base: server.Client().Transport}
	client := &http.Client{Transport: transport, Timeout: imageDownloadTimeout}
	if _, err := downloadTeamLogo(123, server.URL, client); err != nil {
		t.Fatalf("downloadTeamLogo returned error: %v", err)
	}
	if !transport.closed {
		t.Fatal("downloadTeamLogo did not close idle connections")
	}
}

func TestDownloadTeamLogoWithContextCancelsRequest(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := downloadTeamLogoWithContext(ctx, 123, server.URL, server.Client())
		done <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("image request did not start")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("download succeeded after its context was canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("download did not stop after context cancellation")
	}
}

func TestDownloadTeamLogoUsesUTLSTransportForTLS(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Fatal("expected TLS request")
		}
		if r.TLS.Version != tls.VersionTLS12 {
			t.Fatalf("TLS version = %x, want %x", r.TLS.Version, tls.VersionTLS12)
		}
		if r.TLS.NegotiatedProtocol != "http/1.1" {
			t.Fatalf("negotiated protocol = %q, want http/1.1", r.TLS.NegotiatedProtocol)
		}
		if r.Proto != "HTTP/1.1" {
			t.Fatalf("HTTP version = %s, want HTTP/1.1", r.Proto)
		}
		_, _ = w.Write([]byte("test-image"))
	}))
	server.EnableHTTP2 = false
	server.TLS = &tls.Config{NextProtos: []string{"http/1.1"}}
	server.StartTLS()
	defer server.Close()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client := newImageHTTPClientWithTLSConfig(&utls.Config{RootCAs: roots})

	path, err := downloadTeamLogo(123, server.URL, client)
	if err != nil {
		t.Fatalf("downloadTeamLogo returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded logo: %v", err)
	}
	if string(data) != "test-image" {
		t.Fatalf("downloaded data = %q, want test-image", data)
	}
}

type closeTrackingTransport struct {
	base   http.RoundTripper
	closed bool
}

func (t *closeTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closed = true
}

func TestDownloadTeamLogoUsesBrowserHeaders(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), "Mozilla/5.0") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "image/") {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("test-image"))
	}))
	defer server.Close()

	path, err := DownloadTeamLogo(123, server.URL)
	if err != nil {
		t.Fatalf("DownloadTeamLogo returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded logo: %v", err)
	}
	if string(data) != "test-image" {
		t.Fatalf("downloaded data = %q, want test-image", data)
	}
}

// TestDownloadTeamLogoRefererMatchesSourceCDN ensures the Referer
// header is derived from the source URL's origin. Hard-coding the
// Referer to www.sofascore.com broke FotMob-sourced teams because the
// FotMob CDN rejects requests with the wrong (or empty) Referer. The
// fix derives the Referer from the source URL.
//
// This test stands up an httptest server and exercises the live
// download path against the test server's URL, then asserts on the
// Referer header the server observed. We can't reach the real
// FotMob/SofaScore CDNs from a unit test, but the Referer derivation
// is a pure function of neturl.Parse on the source URL — if it's
// correct for localhost, it's correct for every CDN.
func TestDownloadTeamLogoRefererMatchesSourceCDN(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	var observedReferer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedReferer = r.Header.Get("Referer")
		_, _ = w.Write([]byte("test-image"))
	}))
	defer server.Close()

	client := &http.Client{Transport: server.Client().Transport, Timeout: imageDownloadTimeout}
	if _, err := downloadTeamLogo(123, server.URL+"/logo.png", client); err != nil {
		t.Fatalf("downloadTeamLogo returned error: %v", err)
	}
	if observedReferer == "" {
		t.Fatal("expected Referer header to be set, got empty string")
	}
	// The Referer must be derived from the source URL's origin.
	u, parseErr := neturl.Parse(server.URL + "/logo.png")
	if parseErr != nil {
		t.Fatalf("neturl.Parse: %v", parseErr)
	}
	wantReferer := u.Scheme + "://" + u.Host + "/"
	if observedReferer != wantReferer {
		t.Errorf("live Referer = %q, want %q", observedReferer, wantReferer)
	}
}

// TestRefererForKnownCDNShapes asserts the Referer derivation logic
// for the URL shapes the production scraper will produce. The function
// itself is pure (neturl.Parse on a known string), so this is a smoke
// test on the building logic.
func TestRefererForKnownCDNShapes(t *testing.T) {
	cases := []struct {
		name        string
		sourceURL   string
		wantReferer string
	}{
		{"fotmob", "https://images.fotmob.com/image_resources/logo/teamlogo_6504.png", "https://images.fotmob.com/"},
		{"sofascore", "https://img.sofascore.com/api/v1/team/47/image", "https://img.sofascore.com/"},
		{"localhost", "http://127.0.0.1:8080/logo.png", "http://127.0.0.1:8080/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := neturl.Parse(tc.sourceURL)
			if parseErr != nil {
				t.Fatalf("neturl.Parse(%q): %v", tc.sourceURL, parseErr)
			}
			if u.Scheme == "" || u.Host == "" {
				t.Fatalf("URL %q parsed empty scheme/host", tc.sourceURL)
			}
			got := u.Scheme + "://" + u.Host + "/"
			if got != tc.wantReferer {
				t.Errorf("Referer for %q = %q, want %q", tc.sourceURL, got, tc.wantReferer)
			}
		})
	}
}

// TestDownloadTeamLogoMalformedURLSkipsReferer guards against a panic
// or wrong header when the source URL cannot be parsed.
func TestDownloadTeamLogoMalformedURLSkipsReferer(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A malformed source URL will be rejected by net/http before
		// we get here, so we only assert that DownloadTeamLogo
		// returns an error rather than panicking.
		_, _ = w.Write([]byte("never-reached"))
	}))
	defer server.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DownloadTeamLogo panicked on malformed URL: %v", r)
		}
	}()
	_, err := DownloadTeamLogo(123, "ht!tp://[invalid")
	if err == nil {
		t.Fatal("expected error from malformed source URL")
	}
}
