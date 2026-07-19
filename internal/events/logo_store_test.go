package events

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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

func TestDownloadTeamLogoUsesUTLSTransportForTLS(t *testing.T) {
	t.Setenv("IMAGE_STORAGE_PATH", t.TempDir())

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Fatal("expected TLS request")
		}
		if r.TLS.Version != tls.VersionTLS12 {
			t.Fatalf("TLS version = %x, want %x", r.TLS.Version, tls.VersionTLS12)
		}
		if r.ProtoMajor != 1 {
			t.Fatalf("HTTP version = %s, want HTTP/1.1", r.Proto)
		}
		_, _ = w.Write([]byte("test-image"))
	}))
	server.EnableHTTP2 = false
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
