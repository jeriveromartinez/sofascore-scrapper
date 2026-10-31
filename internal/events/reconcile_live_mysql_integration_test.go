//go:build integration

package events

import (
	"context"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func TestLiveEventsFreshness_MariaDB(t *testing.T) {
	db := openMariaDB(t)
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cleanup := func() {
		db.Unscoped().Where("external_match_id LIKE ?", "live-fix-%").Delete(&Event{})
		db.Unscoped().Where("team_id IN ?", []int64{9_100_000_601, 9_100_000_602}).Delete(&Team{})
		db.Unscoped().Where("id = ?", 9_100_000_500).Delete(&tournaments.Tournament{})
		db.Unscoped().Where("tournament_id = ?", 9_100_000_500).Delete(&tournaments.GlobalTournamentConfig{})
	}
	cleanup()
	t.Cleanup(cleanup)

	teams := []Team{
		{TeamId: 9_100_000_601, Name: "Live Fix Home"},
		{TeamId: 9_100_000_602, Name: "Live Fix Away"},
	}
	if err := db.Create(&teams).Error; err != nil {
		t.Fatalf("seed teams: %v", err)
	}

	league := tournaments.Tournament{Model: gorm.Model{ID: 9_100_000_500}, Name: "Live Fix League", Slug: "live-fix-league"}
	if err := db.Create(&league).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: league.ID}).Error; err != nil {
		t.Fatalf("seed selection: %v", err)
	}

	now := time.Now()
	staleStart := now.Add(-48 * time.Hour).UnixMilli()
	freshStart := now.Add(-time.Hour).UnixMilli()
	seed := []Event{
		{ExternalMatchId: "live-fix-stale", Sport: "football", LeagueId: league.ID, StatusType: "inprogress", StartTimestamp: staleStart, ScrapedAt: now.Add(-2 * time.Hour).Unix(), HomeTeamId: 9_100_000_601, AwayTeamId: 9_100_000_602},
		{ExternalMatchId: "live-fix-fresh", Sport: "football", LeagueId: league.ID, StatusType: "inprogress", StartTimestamp: freshStart, ScrapedAt: now.Add(-time.Minute).Unix(), HomeTeamId: 9_100_000_601, AwayTeamId: 9_100_000_602},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed events: %v", err)
	}

	repo := NewRepository(db)
	live, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming: %v", err)
	}
	if len(live) != 1 || live[0].ExternalMatchId != "live-fix-fresh" {
		t.Fatalf("got %d live events, want only live-fix-fresh", len(live))
	}

	closed, err := repo.CloseStaleLive(context.Background(), now)
	if err != nil {
		t.Fatalf("CloseStaleLive: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed %d events, want 1", closed)
	}

	var stale Event
	if err := db.Where("external_match_id = ?", "live-fix-stale").First(&stale).Error; err != nil {
		t.Fatalf("load stale event: %v", err)
	}
	if stale.StatusType != "finished" {
		t.Fatalf("stale status = %q, want finished", stale.StatusType)
	}
}
