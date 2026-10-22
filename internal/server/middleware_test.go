package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
)

// TestSlogLogger_RedactsSensitiveQueryParams asserts that the access
// log never carries a credential. The WebSocket upgrade accepts the
// device credential in ?token=, so the raw query string would leak it
// into the logs.
func TestSlogLogger_RedactsSensitiveQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(observability.NewLogger(slog.LevelInfo, &buf))
	defer slog.SetDefault(previous)

	r := gin.New()
	r.Use(SlogLogger())
	r.GET("/api/app/v1/ws", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/ws?token=super-secret-device-token&page=2", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	out := buf.String()
	if strings.Contains(out, "super-secret-device-token") {
		t.Fatalf("the access log leaked the token: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("the access log did not redact the token: %s", out)
	}
	if !strings.Contains(out, "page=2") {
		t.Fatalf("the access log dropped the harmless parameters: %s", out)
	}
}

// TestRedactQuery_KeepsHarmlessQuery asserts that a query string
// without credentials is logged verbatim.
func TestRedactQuery_KeepsHarmlessQuery(t *testing.T) {
	got := redactQuery("sport=football&limit=6")
	if got != "limit=6&sport=football" {
		t.Fatalf("redactQuery = %q", got)
	}
}
