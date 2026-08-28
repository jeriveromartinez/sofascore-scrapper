//go:build integration

package events

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func createTestEvent(repo *Repository, sofaID int64, statusType string, startTs int64, currentPeriodTs int64) error {
	tm := Team{TeamId: sofaID + 100, Name: "Team"}
	event := Event{
		SofaScoreEventId:            sofaID,
		Sport:                       "football",
		HomeScore:                   0,
		HomeTeamId:                  100,
		AwayScore:                   0,
		AwayTeamId:                  200,
		StartTimestamp:              startTs,
		CurrentPeriodStartTimestamp: currentPeriodTs,
		Slug:                        "test-event",
		LeagueId:                    1,
		StatusType:                  statusType,
	}
	batch := ScrapeBatch{Teams: []Team{tm}, Events: []Event{event}}
	return repo.UpsertScrapeBatch(context.Background(), batch, 500)
}

// Note: TestUpsert_CreatesNewEvent and TestUpsert_UpdatesMutableColumns
// were removed in #52 — they tested the dead Upsert method which has
// been replaced by UpsertScrapeBatch (which is the production path and
// has its own dedicated tests in batch_integration_test.go).

func TestGetCurrentAndUpcoming_ExcludesFinished(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	createTestEvent(repo, 1, "finished", 1710000000000, 1710000000000)
	createTestEvent(repo, 2, "inprogress", 1710000000000, 1710000000000)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})

	events, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	for _, e := range events {
		if e.SofaScoreEventId == 1 {
			t.Error("finished event should not be in current/upcoming")
		}
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestGetCurrentAndUpcoming_LiveByStatusType(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	oldStart := time.Now().Add(-2 * time.Hour).UnixMilli()
	createTestEvent(repo, 1, "inprogress", oldStart, oldStart)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})

	events, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	found := false
	for _, e := range events {
		if e.SofaScoreEventId == 1 {
			found = true
		}
	}
	if !found {
		t.Error("inprogress event with old timestamp should still be live")
	}
}

func TestGetCurrentAndUpcoming_UpcomingByStatusTypeAndStartTimestamp(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	futureStart := time.Now().Add(2 * time.Hour).UnixMilli()
	createTestEvent(repo, 1, "notstarted", futureStart, 0)

	pastStart := time.Now().Add(-1 * time.Hour).UnixMilli()
	createTestEvent(repo, 2, "notstarted", pastStart, 0)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})

	events, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	for _, e := range events {
		if e.SofaScoreEventId == 2 {
			t.Error("notstarted event in the past should not be upcoming")
		}
	}

	foundFuture := false
	for _, e := range events {
		if e.SofaScoreEventId == 1 {
			foundFuture = true
		}
	}
	if !foundFuture {
		t.Error("notstarted event in the future should be upcoming")
	}
}

// Note: TestUpsert_ErrorReturned was removed in #52 — it tested the
// dead Upsert method. The replacement path is UpsertScrapeBatch
// (covered by batch_integration_test.go).

func TestListPage_SportFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	now := time.Now().UnixMilli()
	for i, sport := range []string{"football", "basketball", "football"} {
		if err := db.Create(&Event{
			SofaScoreEventId: int64(2000 + i),
			StartTimestamp:   now + int64(i*3600_000),
			Sport:            sport,
			StatusType:       "notstarted",
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, _, err := repo.ListPage(context.Background(), EventsPageFilter{
		Limit:     10,
		Direction: "asc",
		Sport:     "basketball",
	})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(got) != 1 || got[0].SofaScoreEventId != 2001 {
		t.Fatalf("want 1 basketball event (id 2001), got %d events: %+v", len(got), got)
	}
}

func TestListPage_AllFilters_Combined(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	now := time.Now().UnixMilli()
	league := &tournaments.Tournament{Name: "Primera Division", Slug: "primera"}
	if err := db.Create(league).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	home := &Team{TeamId: 5001, Name: "Barcelona SC"}
	away := &Team{TeamId: 5002, Name: "Emelec"}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("seed home: %v", err)
	}
	if err := db.Create(away).Error; err != nil {
		t.Fatalf("seed away: %v", err)
	}
	if err := db.Create(&Event{
		SofaScoreEventId: 3000, Sport: "football", StatusType: "notstarted",
		StartTimestamp: now + 3600_000, LeagueId: league.ID,
		HomeTeamId: 5001, AwayTeamId: 5002,
	}).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}
	for i, sport := range []string{"basketball", "football", "football"} {
		status := "inprogress"
		if i == 2 {
			status = "finished"
		}
		if err := db.Create(&Event{
			SofaScoreEventId: int64(3001 + i),
			Sport:            sport,
			StatusType:       status,
			StartTimestamp:   now + int64((i+1)*3600_000),
			LeagueId:         league.ID,
			HomeTeamId:       5001, AwayTeamId: 5002,
		}).Error; err != nil {
			t.Fatalf("seed decoy: %v", err)
		}
	}

	got, _, err := repo.ListPage(context.Background(), EventsPageFilter{
		Limit:      10,
		Direction:  "asc",
		Sport:      "football",
		Status:     "notstarted",
		LeagueName: "Primera",
		TeamName:   "Barcelona",
	})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(got) != 1 || got[0].SofaScoreEventId != 3000 {
		t.Fatalf("want only event 3000 (football + notstarted + Primera + Barcelona), got %d events: %+v", len(got), got)
	}
}

func TestListPage_LikeInputEscapesWildcards(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	now := time.Now().UnixMilli()
	teams := []struct {
		id   int64
		name string
	}{
		{6000, "Alpha FC"},
		{6001, "Beta United"},
		{6002, "Team%Special"},
	}
	for i, tm := range teams {
		team := &Team{TeamId: tm.id, Name: tm.name}
		if err := db.Create(team).Error; err != nil {
			t.Fatalf("seed team: %v", err)
		}
		if err := db.Create(&Event{
			SofaScoreEventId: int64(7000 + i),
			Sport:            "football", StatusType: "notstarted",
			StartTimestamp:   now + int64(i*3600_000),
			HomeTeamId:       tm.id,
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	// Per spec §6.1: user-typed '%' must be escaped so it cannot construct a
	// wildcard query. Searching for a literal '%' must match only teams whose
	// name contains a literal '%' (escapeLike wraps input in '%...%').
	got, _, err := repo.ListPage(context.Background(), EventsPageFilter{
		Limit: 10, Direction: "asc", TeamName: "%",
	})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 event (Team%%Special only — %% must NOT act as wildcard), got %d: %+v", len(got), got)
	}
	if got[0].SofaScoreEventId != 7002 {
		t.Fatalf("want event 7002 (Team%%Special), got %d", got[0].SofaScoreEventId)
	}
}

// Note: TestUpsertPersistsLocalLogoURLs was removed in #52 — it tested
// the dead Upsert method. The replacement path is UpsertScrapeBatch
// (covered by TestUpsertScrapeBatch_PersistsLocalLogoURLAndSchedulesRemoteSource
// in batch_integration_test.go).
