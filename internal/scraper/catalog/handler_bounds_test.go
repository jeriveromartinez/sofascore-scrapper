package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_List_ClampsPagination(t *testing.T) {
	r, db := newTestRouter(t)
	repo := NewRepository(db)
	for _, id := range []string{"1", "2", "3"} {
		if err := repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: id, Name: "L" + id, Enabled: true}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	cases := []struct {
		query     string
		wantPage  int
		wantLimit int
		wantRows  int
	}{
		{query: "", wantPage: 1, wantLimit: 50, wantRows: 3},
		{query: "?limit=1000000&page=-3", wantPage: 1, wantLimit: 100, wantRows: 3},
		{query: "?limit=0", wantPage: 1, wantLimit: 1, wantRows: 1},
		{query: "?limit=-7&page=0", wantPage: 1, wantLimit: 1, wantRows: 1},
		{query: "?limit=abc&page=xyz", wantPage: 1, wantLimit: 50, wantRows: 3},
		{query: "?limit=2&page=2", wantPage: 2, wantLimit: 2, wantRows: 1},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/v1/scraper-leagues"+tc.query, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
			var resp struct {
				Data  []json.RawMessage `json:"data"`
				Page  int               `json:"page"`
				Limit int               `json:"limit"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Page != tc.wantPage || resp.Limit != tc.wantLimit || len(resp.Data) != tc.wantRows {
				t.Fatalf("page=%d limit=%d rows=%d, want page=%d limit=%d rows=%d",
					resp.Page, resp.Limit, len(resp.Data), tc.wantPage, tc.wantLimit, tc.wantRows)
			}
		})
	}
}
