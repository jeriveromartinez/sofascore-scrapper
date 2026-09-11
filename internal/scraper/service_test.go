//go:build integration

package scraper

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func setupScraperTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&events.Event{}, &events.Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// Legacy tests preserved as documentation for what Task 7 will delete.
// They reference the removed SofaScore types (APIEvent, TeamApi) and the
// old ToScrapeBatch signature; the bodies are kept here as block comments
// so future readers can see the original test intent.

func TestToScrapeBatch_Deduplication(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		home := TeamApi{ID: 1, Name: "Home"}
		away := TeamApi{ID: 2, Name: "Away"}

		apiEvents := []*APIEvent{
			{
				ID:             100,
				Slug:           "match-1",
				StartTimestamp: 1710000000,
				HomeTeam:       home,
				AwayTeam:       away,
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
			{
				ID:             101,
				Slug:           "match-2",
				StartTimestamp: 1710000000,
				HomeTeam:       home,
				AwayTeam:       away,
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
		}
		apiEvents[0].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[0].Tournament.UniqueTournament.ID = 1
		apiEvents[0].Tournament.UniqueTournament.Name = "Test"
		apiEvents[0].Tournament.UniqueTournament.Slug = "test"
		apiEvents[0].Tournament.UniqueTournament.Category.Name = "Category"
		apiEvents[0].Tournament.UniqueTournament.Category.Slug = "category"

		apiEvents[1].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[1].Tournament.UniqueTournament.ID = 1
		apiEvents[1].Tournament.UniqueTournament.Name = "Test"
		apiEvents[1].Tournament.UniqueTournament.Slug = "test"
		apiEvents[1].Tournament.UniqueTournament.Category.Name = "Category"
		apiEvents[1].Tournament.UniqueTournament.Category.Slug = "category"

		batch := ToScrapeBatch(apiEvents, "football")

		if len(batch.Teams) != 2 {
			t.Errorf("should deduplicate 4 team references to 2, got %d", len(batch.Teams))
		}
		if len(batch.Tournaments) != 1 {
			t.Errorf("should deduplicate 2 tournament references to 1, got %d", len(batch.Tournaments))
		}
		if len(batch.Events) != 2 {
			t.Errorf("should have 2 events, got %d", len(batch.Events))
		}

		for i := range batch.Events {
			if batch.Events[i].HomeTeamModel != nil {
				t.Error("event should not have HomeTeamModel populated")
			}
			if batch.Events[i].AwayTeamModel != nil {
				t.Error("event should not have AwayTeamModel populated")
			}
			if batch.Events[i].League != nil {
				t.Error("event should not have League populated")
			}
		}
	*/
}

func TestToScrapeBatch_SortedOutput(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		apiEvents := []*APIEvent{
			{
				ID:             300,
				Slug:           "z-match",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 30, Name: "Z-Home"},
				AwayTeam:       TeamApi{ID: 10, Name: "A-Away"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "notstarted"},
			},
			{
				ID:             100,
				Slug:           "a-match",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 20, Name: "B-Home"},
				AwayTeam:       TeamApi{ID: 10, Name: "A-Away"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "notstarted"},
			},
		}
		for i := range apiEvents {
			apiEvents[i].Time.CurrentPeriodStartTimestamp = 1710000000
			apiEvents[i].Tournament.UniqueTournament.ID = 2
			apiEvents[i].Tournament.UniqueTournament.Name = "T"
			apiEvents[i].Tournament.UniqueTournament.Slug = "t"
			apiEvents[i].Tournament.UniqueTournament.Category.Name = "c"
			apiEvents[i].Tournament.UniqueTournament.Category.Slug = "c"
		}

		batch := ToScrapeBatch(apiEvents, "football")

		for i := 1; i < len(batch.Teams); i++ {
			if batch.Teams[i].TeamId < batch.Teams[i-1].TeamId {
				t.Errorf("teams not sorted by TeamId")
			}
		}
		for i := 1; i < len(batch.Events); i++ {
			if batch.Events[i].SofaScoreEventId < batch.Events[i-1].SofaScoreEventId {
				t.Errorf("events not sorted by SofaScoreEventId")
			}
		}
	*/
}

func TestToScrapeBatch_StructureRoundTrip(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		db := setupScraperTestDB(t)
		repo := events.NewRepository(db)

		apiEvents := []*APIEvent{
			{
				ID:             1,
				Slug:           "round-trip",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 1, Name: "H"},
				AwayTeam:       TeamApi{ID: 2, Name: "A"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
		}
		apiEvents[0].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[0].Tournament.UniqueTournament.ID = 1
		apiEvents[0].Tournament.UniqueTournament.Name = "League"
		apiEvents[0].Tournament.UniqueTournament.Slug = "league"
		apiEvents[0].Tournament.UniqueTournament.Category.Name = "cat"
		apiEvents[0].Tournament.UniqueTournament.Category.Slug = "cat"

		batch := ToScrapeBatch(apiEvents, "football")

		if err := repo.UpsertScrapeBatch(nil, batch, 500); err != nil {
			t.Fatalf("UpsertScrapeBatch: %v", err)
		}

		var teamCount int64
		db.Model(&events.Team{}).Count(&teamCount)
		if teamCount != 2 {
			t.Errorf("expected 2 teams, got %d", teamCount)
		}

		var eventCount int64
		db.Model(&events.Event{}).Count(&eventCount)
		if eventCount != 1 {
			t.Errorf("expected 1 event, got %d", eventCount)
		}

		var tourCount int64
		db.Model(&tournaments.Tournament{}).Count(&tourCount)
		if tourCount != 1 {
			t.Errorf("expected 1 tournament, got %d", tourCount)
		}
	*/
}

func TestToScrapeBatch_EmptyInput(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		batch := ToScrapeBatch(nil, "football")

		if len(batch.Teams) != 0 {
			t.Errorf("expected 0 teams, got %d", len(batch.Teams))
		}
		if len(batch.Tournaments) != 0 {
			t.Errorf("expected 0 tournaments, got %d", len(batch.Tournaments))
		}
		if len(batch.Events) != 0 {
			t.Errorf("expected 0 events, got %d", len(batch.Events))
		}
	*/
}

type serviceTestFakeSource struct {
	matches []Match
	err     error
}

func (f *serviceTestFakeSource) Name() string { return "fake" }
func (f *serviceTestFakeSource) ScheduledEvents(_ context.Context, _ LeagueRef, _ time.Time) ([]Match, error) {
	return f.matches, f.err
}
func (f *serviceTestFakeSource) SearchLeagues(_ context.Context, _ string) ([]LeagueSearchResult, error) {
	return nil, nil
}

// serviceTestFakeCatalog is a CatalogSource stub used by the integration
// test now that the memory-catalog package has been removed. It avoids
// an import cycle with internal/scraper/catalog (which imports this
// package for LeagueRef).
type serviceTestFakeCatalog struct {
	leagues []LeagueRef
}

func (c *serviceTestFakeCatalog) ActiveLeagues(_ context.Context) ([]LeagueRef, error) {
	out := make([]LeagueRef, len(c.leagues))
	copy(out, c.leagues)
	return out, nil
}

func TestService_ScrapeToday_UpsertsFromSource(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &serviceTestFakeSource{
		matches: []Match{
			{
				Source:         "fake",
				SourceMatchId:  "1",
				Slug:           "x",
				StartTimestamp: time.Now(),
				Status:         MatchStatus{Type: "scheduled"},
				HomeTeam:       Team{SourceId: 1, Name: "H"},
				AwayTeam:       Team{SourceId: 2, Name: "A"},
				League:         LeagueRef{Source: "fake", SourceLeagueId: "47", Name: "L"},
			},
		},
	}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{{Source: "fake", SourceLeagueId: "47", Name: "L"}}}
	svc, err := NewService(repo, src, cat, 100, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	svc.ScrapeToday(context.Background(), time.Now())

	var count int64
	if err := db.Model(&events.Event{}).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event, got %d", count)
	}
}
