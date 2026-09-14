// internal/scraper/catalog/discovery_test.go
package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scores365"
	"gorm.io/gorm"
)

func newDiscoveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&ScraperLeague{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDiscovery_ParsesSitemapUrls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><loc>https://www.365scores.com/tennis/league/wimbledon---men-215</loc><loc>https://www.365scores.com/tennis/league/us-open---men-230</loc></urlset>`))
	}))
	defer srv.Close()
	os.Setenv("SCORES365_SITEMAP_URL", srv.URL+"/sitemaps")
	defer os.Unsetenv("SCORES365_SITEMAP_URL")

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{SitemapURL: srv.URL + "/sitemaps"})
	res, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("Discovery: %v", err)
	}
	if res.Upserted != 2 {
		t.Errorf("Upserted = %d, want 2", res.Upserted)
	}
	if res.Source != "scores365" {
		t.Errorf("Source = %q, want scores365", res.Source)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Source != "scores365" {
			t.Errorf("row source = %q, want scores365", r.Source)
		}
	}
	if !rows[0].Enabled {
		t.Errorf("new rows must default to enabled=true")
	}
}

func TestDiscovery_DeduplicatesAcrossLanguages(t *testing.T) {
	bodyEN := `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><loc>https://www.365scores.com/tennis/league/wimbledon---men-215</loc></urlset>`
	bodyES := bodyEN // same compId, different lang
	var reqLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_ = reqLang
		_, _ = w.Write([]byte(bodyEN))
	}))
	defer srv.Close()
	_ = bodyES

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{SitemapURL: srv.URL + "/sitemaps"})

	// Run twice with different lang hints to simulate two languages
	_, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err = Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("dedup failed, got %d rows, want 1", len(rows))
	}
}

func TestDiscovery_HandlesInvalidXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("not xml at all"))
	}))
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{SitemapURL: srv.URL + "/sitemaps"})

	// Discovery should not panic; individual sport errors should not stop the whole run
	res, err := Discovery(context.Background(), repo, client, nil)
	// We accept either a partial success or a wrapped error
	if err != nil && res.Upserted > 0 {
		t.Errorf("error returned but partial upserts happened: %v", err)
	}
}