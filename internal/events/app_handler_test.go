package events

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
)

func TestParseCurrentEventsLimit(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLimit int
		wantOK    bool
	}{
		{name: "empty string defaults to 6", input: "", wantLimit: 6, wantOK: true},
		{name: "valid 1", input: "1", wantLimit: 1, wantOK: true},
		{name: "valid 4", input: "4", wantLimit: 4, wantOK: true},
		{name: "valid 6 (max)", input: "6", wantLimit: 6, wantOK: true},
		{name: "zero rejected", input: "0", wantLimit: 0, wantOK: false},
		{name: "negative rejected", input: "-1", wantLimit: 0, wantOK: false},
		{name: "above max rejected", input: "7", wantLimit: 0, wantOK: false},
		{name: "way above max rejected", input: "100", wantLimit: 0, wantOK: false},
		{name: "non-numeric rejected", input: "abc", wantLimit: 0, wantOK: false},
		{name: "numeric with letters rejected", input: "4x", wantLimit: 0, wantOK: false},
		{name: "float rejected", input: "3.5", wantLimit: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCurrentEventsLimit(tt.input)
			if got != tt.wantLimit {
				t.Errorf("limit = %d, want %d", got, tt.wantLimit)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestHandleGetCurrentEventsInvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []string{"0", "-1", "7", "100", "abc", "4x", "3.5"}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/current-events?limit="+q, nil)
			c.Set("device", devices.Device{})

			h := &AppHandler{}
			h.handleGetCurrentEvents(c)

			if w.Code != http.StatusBadRequest {
				t.Errorf("limit=%s: status = %d, want %d", q, w.Code, http.StatusBadRequest)
			}
		})
	}
}
