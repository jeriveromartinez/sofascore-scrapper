// internal/scraper/catalog/model_test.go
package catalog

import "testing"

func TestScraperLeague_TableName(t *testing.T) {
	var s ScraperLeague
	if s.TableName() != "scraper_leagues" {
		t.Fatalf("table: %q", s.TableName())
	}
}
