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
	if err := db.Exec("PRAGMA case_sensitive_like = ON").Error; err != nil {
		t.Fatalf("failed to set case_sensitive_like: %v", err)
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
		HomeTeamModel:               &tm,
	}
	return repo.Upsert(context.Background(), []Event{event}, "football")
}

func TestUpsert_CreatesNewEvent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	err := createTestEvent(repo, 1, "inprogress", 1710000000000, 1710000000000)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	var count int64
	db.Model(&Event{}).Where("sofa_score_event_id = ?", 1).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}
}

func TestUpsert_UpdatesMutableColumns(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	createTestEvent(repo, 1, "notstarted", 1710000000000, 0)
	oldScraped := int64(0)
	db.Model(&Event{}).Where("sofa_score_event_id = ?", 1).Pluck("scraped_at", &oldScraped)

	time.Sleep(time.Second)

	homeTeam := Team{TeamId: 999, Name: "Team Updated"}
	awayTeam := Team{TeamId: 888, Name: "Away Updated"}
	updatedEvent := Event{
		SofaScoreEventId:            1,
		Sport:                       "basketball",
		HomeScore:                   10,
		HomeTeamId:                  999,
		AwayScore:                   5,
		AwayTeamId:                  888,
		StartTimestamp:              1720000000000,
		CurrentPeriodStartTimestamp: 1720000000000,
		Slug:                        "updated-slug",
		LeagueId:                    2,
		StatusType:                  "finished",
		HomeTeamModel:               &homeTeam,
		AwayTeamModel:               &awayTeam,
	}
	if err := repo.Upsert(context.Background(), []Event{updatedEvent}, "basketball"); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	var event Event
	db.Where("sofa_score_event_id = ?", 1).First(&event)

	if event.Sport != "basketball" {
		t.Errorf("sport: want basketball, got %s", event.Sport)
	}
	if event.HomeScore != 10 {
		t.Errorf("home_score: want 10, got %d", event.HomeScore)
	}
	if event.AwayScore != 5 {
		t.Errorf("away_score: want 5, got %d", event.AwayScore)
	}
	if event.HomeTeamId != 999 {
		t.Errorf("home_team_id: want 999, got %d", event.HomeTeamId)
	}
	if event.AwayTeamId != 888 {
		t.Errorf("away_team_id: want 888, got %d", event.AwayTeamId)
	}
	if event.StartTimestamp != 1720000000000 {
		t.Errorf("start_timestamp: want 1720000000000, got %d", event.StartTimestamp)
	}
	if event.CurrentPeriodStartTimestamp != 1720000000000 {
		t.Errorf("current_period_start_timestamp: want 1720000000000, got %d", event.CurrentPeriodStartTimestamp)
	}
	if event.Slug != "updated-slug" {
		t.Errorf("slug: want updated-slug, got %s", event.Slug)
	}
	if event.LeagueId != 2 {
		t.Errorf("league_id: want 2, got %d", event.LeagueId)
	}
	if event.StatusType != "finished" {
		t.Errorf("status_type: want finished, got %s", event.StatusType)
	}
	if event.ScrapedAt == 0 {
		t.Errorf("scraped_at should be set")
	}
}

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

func TestUpsert_ErrorReturned(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	event := Event{SofaScoreEventId: 1, Sport: "football"}
	if err := repo.Upsert(context.Background(), []Event{event}, "football"); err != nil {
		t.Fatalf("first Upsert should succeed: %v", err)
	}
	if err := repo.Upsert(context.Background(), []Event{event}, "football"); err != nil {
		t.Errorf("upsert with conflict should not error: %v", err)
	}
}

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
	for i, name := range []string{"Alpha FC", "Beta United", "Alphas"} {
		tm := &Team{TeamId: int64(6000 + i), Name: name}
		if err := db.Create(tm).Error; err != nil {
			t.Fatalf("seed team: %v", err)
		}
		if err := db.Create(&Event{
			SofaScoreEventId: int64(7000 + i),
			Sport:            "football", StatusType: "notstarted",
			StartTimestamp:   now + int64(i*3600_000),
			HomeTeamId:       tm.TeamId,
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	got, _, err := repo.ListPage(context.Background(), EventsPageFilter{
		Limit: 10, Direction: "asc", TeamName: "A%",
	})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events (Alpha FC + Alphas), got %d: %+v", len(got), got)
	}
}

func TestUpsertPersistsLocalLogoURLs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	repo.scheduleLogo = func(_ *gorm.DB, _ int64, _ string) {}

	const remoteURL = "https://img.sofascore.com/api/v1/team/10/image"
	home := &Team{TeamId: 10, Name: "Home", LogoUrl: remoteURL}
	away := &Team{TeamId: 20, Name: "Away", LogoUrl: remoteURL}
	event := Event{
		SofaScoreEventId: 1,
		HomeTeamId:       10,
		AwayTeamId:       20,
		HomeTeamModel:    home,
		AwayTeamModel:    away,
		StatusType:       "notstarted",
	}
	if err := repo.Upsert(context.Background(), []Event{event}, "football"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var teams []Team
	if err := db.Order("team_id").Find(&teams).Error; err != nil {
		t.Fatalf("load teams: %v", err)
	}
	if len(teams) != 2 || teams[0].LogoUrl != "/teams/logo/10" || teams[1].LogoUrl != "/teams/logo/20" {
		t.Fatalf("persisted teams = %#v", teams)
	}
}
