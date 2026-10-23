package fotmob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestClient_DayMatches_BodyReadErrorPropagated is the regression
// test for the discarded io.ReadAll error: the client used to swallow
// a truncated body and hand an empty payload to the JSON decoder, so
// the failure surfaced as a parse error (or, worse, as an empty day).
func TestClient_DayMatches_BodyReadErrorPropagated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("hijacking unsupported by the test server")
			return
		}
		conn, buf, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		// Announce more bytes than are sent, then hang up: the
		// client sees a truncated body.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 512\r\n\r\n")
		_, _ = buf.WriteString(`{"leagues":[]}`)
		buf.Flush()
	}))
	defer server.Close()

	c := NewClient(ClientConfig{BaseURL: server.URL, Timezone: "Europe/Paris", MaxRetries: 1})
	_, err := c.dayMatches(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected the truncated body to be reported as an error")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("error = %v, want it to mention the body read failure", err)
	}
}
