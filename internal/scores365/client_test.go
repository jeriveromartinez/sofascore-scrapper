package scores365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_FetchGames_ParsesRealShape(t *testing.T) {
	body := `{"LastUpdateID":12345,"CurrentDate":"14-09-2026","CurrentTimeUTC":"09/14/2026 02:00:00","TTL":10,"Games":[{"ID":4620228,"Comp":438,"SID":7,"Stage":2,"STID":82,"GT":80,"Completion":88.89,"STime":"13-09-2026 23:20","ETime":"14-09-2026 02:35","Scrs":[4,6],"Comps":[{"ID":7421,"Name":"San Francisco Giants","SName":"Giants","CID":323,"Color":"#FD5A1E"},{"ID":7420,"Name":"San Diego Padres","SName":"Padres","CID":323,"Color":"#2F241D"}],"Winner":-1,"IsFinished":false}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" {
			t.Errorf("missing Referer header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	feed, err := c.FetchGames(context.Background(), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchGames: %v", err)
	}
	if len(feed.Games) != 1 {
		t.Fatalf("got %d games, want 1", len(feed.Games))
	}
	g := feed.Games[0]
	if g.ID != 4620228 || g.Comp != 438 || g.SID != 7 {
		t.Errorf("game fields wrong: %+v", g)
	}
	if g.Comps[0].ID != 7421 || g.Comps[0].Name != "San Francisco Giants" {
		t.Errorf("home team wrong: %+v", g.Comps[0])
	}
}

func TestClient_FetchGames_RetryOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Games":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	if _, err := c.FetchGames(context.Background(), time.Now()); err != nil {
		t.Fatalf("FetchGames: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("hits = %d, want 3 (2 retries then success)", got)
	}
}

func TestClient_CacheTTL(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Games":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := c.FetchGames(context.Background(), date); err != nil {
			t.Fatalf("FetchGames: %v", err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (cache should hit on 2nd and 3rd call)", got)
	}
}

func TestClient_FetchSitemap_ParsesURLs(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<loc>https://www.365scores.com/tennis/league/wimbledon---men-215</loc>
<loc>https://www.365scores.com/tennis/league/us-open---men-230</loc>
<loc>https://www.365scores.com/tennis/league/not-a-league-url</loc>
</urlset>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(Options{SitemapURL: srv.URL + "/sitemaps"})
	got, err := c.FetchSitemap(context.Background(), "en", "tennis")
	if err != nil {
		t.Fatalf("FetchSitemap: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d leagues, want 2 (filtered out not-a-league-url)", len(got))
	}
	if got[0].SourceLeagueId != "215" || got[1].SourceLeagueId != "230" {
		t.Errorf("compIds wrong: %+v", got)
	}
	if got[0].Name != "Wimbledon Men" || got[1].Name != "Us Open Men" {
		t.Errorf("names wrong: %+v", got)
	}
	if got[0].NameForURL != "wimbledon---men" {
		t.Errorf("NameForURL wrong: %+v", got[0])
	}
}

func TestClient_FetchSitemap_NotFoundReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := NewClient(Options{SitemapURL: srv.URL + "/sitemaps"})
	got, err := c.FetchSitemap(context.Background(), "en", "curling")
	if err != nil {
		t.Fatalf("FetchSitemap: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil for 404", got)
	}
}

func TestParseSitemap_HumanizesSlug(t *testing.T) {
	body := []byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><loc>https://www.365scores.com/basketball/usa/nba-47</loc></urlset>`)
	leagues, err := parseSitemap(body, "basketball")
	if err != nil {
		t.Fatalf("parseSitemap: %v", err)
	}
	if len(leagues) != 1 || leagues[0].Name != "Usa Nba" || leagues[0].SourceLeagueId != "47" {
		t.Errorf("got %+v", leagues)
	}
}

func TestClient_FetchSitemap_WrappedShape(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://www.365scores.com/basketball/usa/nba-47</loc></url>
<url><loc>https://www.365scores.com/basketball/spain/acb-50</loc></url>
<url><loc>https://www.365scores.com/tennis/league/wimbledon---men-215</loc></url>
</urlset>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(Options{SitemapURL: srv.URL + "/sitemaps"})
	got := make([]League, 0, 3)
	for _, sport := range []string{"basketball", "tennis"} {
		leagues, err := c.FetchSitemap(context.Background(), "en", sport)
		if err != nil {
			t.Fatalf("FetchSitemap(%s): %v", sport, err)
		}
		got = append(got, leagues...)
	}
	if len(got) != 3 {
		t.Fatalf("got %d leagues, want 3 (nba + acb + wimbledon)", len(got))
	}
	if got[0].Country != "usa" {
		t.Errorf("got[0].Country = %q, want %q (raw slug)", got[0].Country, "usa")
	}
	if got[1].Country != "spain" {
		t.Errorf("got[1].Country = %q, want %q (raw slug)", got[1].Country, "spain")
	}
}

func TestClient_FetchSitemap_CountryRawSlug(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://www.365scores.com/basketball/usa/nba-47</loc></url>
</urlset>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(Options{SitemapURL: srv.URL + "/sitemaps"})
	got, err := c.FetchSitemap(context.Background(), "en", "basketball")
	if err != nil {
		t.Fatalf("FetchSitemap: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d leagues, want 1", len(got))
	}
	if got[0].Country != "usa" {
		t.Errorf("got[0].Country = %q, want %q (raw slug, not humanized)", got[0].Country, "usa")
	}
}

// silence linter when strconv is unused on some build tags
var _ = strconv.Itoa
var _ = strings.TrimSpace
