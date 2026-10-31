package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRunURLPrintsThePanelURLOfThePackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/devices/url/mx.qchotv.tvapp", r.URL.Path)

		out, err := proto.Marshal(&pb.DeviceUrl{Url: "https://iptvdev.tumejorcasa.xyz"})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"url", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "https://iptvdev.tumejorcasa.xyz")
}

// DeviceApi.iptvPanelUrl returns the default panel when the request or
// the decode fails.
func TestRunURLFallsBackToTheDefaultPanelWhenTheRequestFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"url", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), DefaultPanelURL)
	assert.Contains(t, stderr.String(), "boom")
}

func TestRunURLFallsBackToTheDefaultPanelOnAnEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"url", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), DefaultPanelURL)
}

// UpdateApi.checkForUpdate returns null when the call fails; the CLI
// reports it without failing the whole session.
func TestRunUpdateReportsNoDataWithoutFailing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "sin informacion de actualizacion")
}

func TestRunURLJSONModePrintsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := proto.Marshal(&pb.DeviceUrl{Url: "https://iptvdev.tumejorcasa.xyz"})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"url", "--base-url", srv.URL, "--json"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), `"url"`)
	assert.Contains(t, stdout.String(), "https://iptvdev.tumejorcasa.xyz")
}

func TestRunDownloadWritesTheFileToTheGivenPath(t *testing.T) {
	payload := []byte("apk-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/apk/download/token-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.Header().Set("Content-Disposition", `attachment; filename="app-1.0.30.apk"`)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "app.apk")
	var stdout, stderr bytes.Buffer
	code := run([]string{"download", "token-1", "--base-url", srv.URL, "--out", out}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, payload, written)
	assert.Contains(t, stdout.String(), out)
}

func TestRunDownloadUsesTheFilenameTheServerSuggests(t *testing.T) {
	payload := []byte("apk-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="app-1.0.30.apk"`)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"download", "token-1", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	written, err := os.ReadFile(filepath.Join(dir, "app-1.0.30.apk"))
	require.NoError(t, err)
	assert.Equal(t, payload, written)
}

func TestRunDownloadWithoutATokenFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"download", "--base-url", "http://127.0.0.1:1"}, &stdout, &stderr)

	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "falta")
}

func TestRunDownloadReportsTheServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"APK version not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"download", "token-1", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "APK version not found")
}

func TestRunLogoWritesTheFileToTheGivenPath(t *testing.T) {
	payload := []byte{0x89, 'P', 'N', 'G'}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/teams/logo/123", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "logo.png")
	var stdout, stderr bytes.Buffer
	code := run([]string{"logo", "123", "--base-url", srv.URL, "--out", out}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, payload, written)
}

func TestRunLogoNamesTheFileAfterTheTeam(t *testing.T) {
	payload := []byte{0x89, 'P', 'N', 'G'}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"logo", "123", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	written, err := os.ReadFile(filepath.Join(dir, "team-123.png"))
	require.NoError(t, err)
	assert.Equal(t, payload, written)
}

func TestRunLogoWithoutATeamIDFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"logo", "--base-url", "http://127.0.0.1:1"}, &stdout, &stderr)

	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "falta")
}
