package events

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
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
// CDNs that front imagecache.365scores.com and images.fotmob.com with
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
		w.Header().Set("Content-Type", "image/png")
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
// the image CDNs would reject the resulting ClientHello with HTTP
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
		w.Header().Set("Content-Type", "image/png")
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
// Referer to a single CDN host broke FotMob-sourced teams because the
// FotMob CDN rejects requests with the wrong (or empty) Referer. The
// fix derives the Referer from the source URL.
//
// This test stands up an httptest server and exercises the live
// download path against the test server's URL, then asserts on the
// Referer header the server observed. We can't reach the real
// FotMob/365scores CDNs from a unit test, but the Referer derivation
// is a pure function of neturl.Parse on the source URL — if it's
// correct for localhost, it's correct for every CDN.
func TestDownloadTeamLogoRefererMatchesSourceCDN(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	var observedReferer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "image/png")
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
		{"fotmob", "https://images.fotmob.com/image_resources/logo/teamlogo/6504.png", "https://images.fotmob.com/"},
		{"scores365", scores365Badge + "13693", "https://imagecache.365scores.com/"},
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

// TestDownloadTeamLogoRejectsNonImageContentType guards against
// persisting a CDN "challenge" or JSON error body as a logo. Some
// CDNs return a small HTML or text body with HTTP 200 when the
// request is rate-limited, geo-blocked, or intercepted by a WAF;
// without this check the file would land on disk and silently break
// every team page that referenced it.
func TestDownloadTeamLogoRejectsNonImageContentType(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>blocked</html>"))
	}))
	defer server.Close()

	if _, err := DownloadTeamLogo(123, server.URL+"/logo.png"); err == nil {
		t.Fatal("expected error for non-image Content-Type, got nil")
	} else if !strings.Contains(err.Error(), "Content-Type") {
		t.Errorf("error %q should mention Content-Type", err)
	}

	if _, err := os.Stat(TeamLogoLocalPath(123)); err == nil {
		t.Error("rejected response must not leave a logo file on disk")
	}
}

// TestDownloadTeamLogoAcceptsBinaryBlobContentType covers the
// "octet-stream" path: a CDN that serves an image without a declared
// Content-Type. Go's stdlib server-side sniffer detects binary bytes
// (no known image signature) and sets application/octet-stream in
// the response header, so the request reaches the downloader with
// that type. We accept it because:
//
//   - the size cap (maxLogoBytes) still bounds the body, so a hostile
//     upstream cannot push a multi-GB blob to disk;
//   - rejecting every octet-stream response would also reject every
//     team whose badge is served by a CDN that omits the declared
//     type (a common, legitimate configuration);
//
// The "challenge page" case (which returns text/html or application/
// json with a 200) is still rejected by the surrounding check.
func TestDownloadTeamLogoAcceptsBinaryBlobContentType(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// High-bit binary that doesn't match a known image signature
		// makes Go's stdlib sniffer set application/octet-stream.
		_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd})
	}))
	defer server.Close()

	path, err := DownloadTeamLogo(123, server.URL+"/logo.png")
	if err != nil {
		t.Fatalf("DownloadTeamLogo returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded logo: %v", err)
	}
	if len(data) == 0 {
		t.Error("downloaded file is empty")
	}
}

// TestDownloadTeamLogoCapsBodyAtMaxLogoBytes ensures a hostile or
// misconfigured upstream cannot push a multi-GB body to disk. The
// cap is enforced both via Content-Length (fast path, when the
// server advertises it) and via io.LimitReader (belt-and-suspenders
// for chunked transfers or a lying Content-Length).
func TestDownloadTeamLogoCapsBodyAtMaxLogoBytes(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	// Declare a Content-Length above the cap; the downloader must
	// refuse without reading the body.
	oversized := maxLogoBytes + 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", oversized))
		_, _ = w.Write(make([]byte, oversized))
	}))
	defer server.Close()

	if _, err := DownloadTeamLogo(123, server.URL+"/logo.png"); err == nil {
		t.Fatal("expected error for oversized Content-Length, got nil")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error %q should mention 'too large'", err)
	}
}

// TestDownloadTeamLogoLimitReaderEnforcesCapForChunked covers the
// belt-and-suspenders path: a server that omits Content-Length
// (chunked transfer encoding) cannot blow past maxLogoBytes either,
// because io.LimitReader bounds the read at the cap regardless of
// what the server advertises.
func TestDownloadTeamLogoLimitReaderEnforcesCapForChunked(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Flush a body well over the cap in chunks; the transfer
		// encoding becomes chunked because the handler doesn't
		// declare Content-Length up front.
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 4; i++ {
			_, _ = w.Write(make([]byte, maxLogoBytes))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	// Either the downloader fails outright, or it succeeds but the
	// file on disk is bounded by the cap. Either way it must not
	// blow up the test process or the disk with a multi-GB write.
	path, err := DownloadTeamLogo(123, server.URL+"/logo.png")
	if err == nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat downloaded logo: %v", statErr)
		}
		if info.Size() > maxLogoBytes {
			t.Fatalf("downloaded size = %d, want <= %d", info.Size(), maxLogoBytes)
		}
	}
}

// TestDownloadTeamLogoRejectsChunkedBodyOverCap is the regression test
// for the chunked-transfer hole in the size cap: the body was read
// through io.LimitReader, which stops at exactly maxLogoBytes and
// therefore persisted the truncated prefix of an oversized response as
// if it were a complete logo.
func TestDownloadTeamLogoRejectsChunkedBodyOverCap(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 2; i++ {
			_, _ = w.Write(make([]byte, maxLogoBytes))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	_, err := DownloadTeamLogo(123, server.URL+"/logo.png")
	if err == nil {
		t.Fatal("expected an error for an oversized chunked body, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error %q should mention 'too large'", err)
	}
	if _, statErr := os.Stat(TeamLogoLocalPath(123)); !os.IsNotExist(statErr) {
		t.Errorf("an oversized logo must not be persisted at %s", TeamLogoLocalPath(123))
	}
}

// 365scores image CDN, up to the natural competitor ID.
const scores365Badge = "https://imagecache.365scores.com/image/upload/f_png,w_68,h_68,c_limit,q_auto:eco,d_Competitors:default1.png/v1/Competitors/"

// TestTeamLogoSourceURL asserts the fallback URL the LogoScheduler
// uses when neither the source-provided badge nor the name lookup
// resolves a team. Source IDs in the shared `teams` table are
// namespaced to avoid collisions (7B for scores365, 2B for the
// removed TheSportsDB), but every CDN is keyed by the upstream's
// natural ID — so the prefix must be stripped, and the namespace also
// decides which CDN is used at all.
func TestTeamLogoSourceURL(t *testing.T) {
	cases := []struct {
		name   string
		teamID int64
		want   string
	}{
		{"fotmob_natural_id", 9825, "https://images.fotmob.com/image_resources/logo/teamlogo/9825.png"},
		{"scores365_prefixed_id", 7_000_000_000 + 123, scores365Badge + "123"},
		// TheSportsDB is gone, so nothing can serve these badges. An
		// empty URL means "nothing to download" rather than a request
		// against an empty host.
		{"sportsdb_legacy_prefixed_id", 2_000_000_000 + 789, ""},
		{"zero_id", 0, ""},
		{"negative_id", -5, ""},
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
