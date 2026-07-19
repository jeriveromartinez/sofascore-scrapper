package events

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
