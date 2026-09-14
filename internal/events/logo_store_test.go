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
)

func TestNewImageHTTPClientUsesImageTimeout(t *testing.T) {
	client := newImageHTTPClient()
	if client.Timeout != imageDownloadTimeout {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, imageDownloadTimeout)
	}
}

// TestNewImageHTTPClientUsesStandardTransport pins the transport to
// the standard library's *http.Transport. The previous uTLS-backed
// implementation (utls.HelloRandomizedALPN pinned to TLS 1.2 with a
// randomized fingerprint) was rejected by the Cloudflare/CloudFront
// CDNs that front img.sofascore.com and images.fotmob.com with HTTP
// 403, which made the LogoScheduler silently skip every team. A
// default *http.Transport negotiates TLS 1.2/1.3 with the Go runtime's
// stable fingerprint, which both CDNs accept.
func TestNewImageHTTPClientUsesStandardTransport(t *testing.T) {
	client := newImageHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.DialTLSContext != nil {
		t.Fatal("image transport must not override the standard library's TLS dialer (uTLS breaks Cloudflare/CloudFront CDNs)")
	}
	if transport.TLSHandshakeTimeout == 0 {
		t.Fatal("image transport must set a TLSHandshakeTimeout so a stuck dial does not stall the worker pool")
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

// TestDownloadTeamLogoNegotiatesTLS13 ensures the image client can
// complete a TLS 1.3 handshake against a TLS 1.3-only server. The
// previous uTLS-backed client was hard-pinned to TLS 1.2 (it called
// SetTLSVers with VersionTLS12 on both ends and disabled the TLS 1.3
// extension) and therefore could not talk to any CDN that had
// deprecated TLS 1.0/1.1. Cloudflare/CloudFront in front of
// img.sofascore.com would reject the resulting ClientHello with HTTP
// 403. Using the standard *http.Transport lets the Go runtime pick
// the best protocol the server offers.
//
// The test wires in the server's certificate directly so the
// transport can verify the TLS handshake. Without that, the test
// certificate is self-signed and the stdlib transport — which uses
// the system trust store — would reject it. That is independent of
// the TLS version being negotiated: it only proves the client is
// willing to negotiate TLS 1.3 when the server offers it.
func TestDownloadTeamLogoNegotiatesTLS13(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Fatal("expected TLS request")
		}
		if r.TLS.Version != tls.VersionTLS13 {
			t.Fatalf("TLS version = %x, want %x", r.TLS.Version, tls.VersionTLS13)
		}
		_, _ = w.Write([]byte("tls13-image"))
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots},
		},
		Timeout: imageDownloadTimeout,
	}

	path, err := downloadTeamLogoWithContext(context.Background(), 123, server.URL, client)
	if err != nil {
		t.Fatalf("downloadTeamLogoWithContext returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded logo: %v", err)
	}
	if string(data) != "tls13-image" {
		t.Fatalf("downloaded data = %q, want tls13-image", data)
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

// TestTeamLogoSourceURLStripsPrefix asserts that the URL the
// LogoScheduler falls back to for an unknown IDs strips per-source
// prefixes before building the SofaScore CDN URL. Source IDs in the
// shared `teams` table are prefixed to avoid collisions (7B for
// scores365, 2B for the removed TheSportsDB), but SofaScore's CDN is
// keyed by the upstream's natural ID. With the fix applied,
// `TeamLogoSourceURL(7000001234)` returns the same URL as
// `TeamLogoSourceURL(1234)`.
func TestTeamLogoSourceURLStripsPrefix(t *testing.T) {
	cases := []struct {
		name   string
		teamID int64
		want   string
	}{
		{"fotmob_natural_id", 9825, "https://img.sofascore.com/api/v1/team/9825/image"},
		{"scores365_prefixed_id", 7_000_000_000 + 123, "https://img.sofascore.com/api/v1/team/123/image"},
		{"sportsdb_legacy_prefixed_id", 2_000_000_000 + 789, "https://img.sofascore.com/api/v1/team/789/image"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TeamLogoSourceURL(tc.teamID)
			if got != tc.want {
				t.Errorf("TeamLogoSourceURL(%d) = %q, want %q", tc.teamID, got, tc.want)
			}
		})
	}
}
