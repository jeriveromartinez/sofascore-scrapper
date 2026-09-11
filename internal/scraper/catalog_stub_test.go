package scraper

import (
	"context"
	"testing"
)

func TestCatalogStub_ActiveLeagues(t *testing.T) {
	stub := NewMemoryCatalog([]LeagueRef{
		{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League"},
	})
	leagues, err := stub.ActiveLeagues(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(leagues) != 1 || leagues[0].SourceLeagueId != "47" {
		t.Fatalf("unexpected: %+v", leagues)
	}
}
