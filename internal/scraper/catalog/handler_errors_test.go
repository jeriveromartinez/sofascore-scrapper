package catalog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_InternalErrorsDoNotLeakDetails(t *testing.T) {
	r, db := newTestRouter(t)
	if err := db.Migrator().DropTable(&ScraperLeague{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	createBody := `{"source":"fotmob","source_league_id":"47","name":"PL","country":"GB","sport":"football"}`
	cases := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/admin/v1/scraper-leagues", ""},
		{http.MethodPost, "/api/admin/v1/scraper-leagues", createBody},
		{http.MethodGet, "/api/admin/v1/scraper-leagues/search?q=premier", ""},
		{http.MethodGet, "/api/admin/v1/scraper-leagues/1", ""},
		{http.MethodPatch, "/api/admin/v1/scraper-leagues/1", `{"name":"x"}`},
		{http.MethodDelete, "/api/admin/v1/scraper-leagues/1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d body = %s, want 500", w.Code, w.Body.String())
			}
			if body := w.Body.String(); strings.Contains(body, "scraper_leagues") || strings.Contains(body, "no such table") {
				t.Fatalf("500 body leaks the underlying error: %s", body)
			}
		})
	}
}
