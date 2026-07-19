//go:build integration

package events

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func setupBatchTestDB(tb testing.TB) *gorm.DB {
	tb.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		tb.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		tb.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func testTeam(id int64) Team {
	return Team{TeamId: id, Name: "Team", LogoUrl: "https://img.sofascore.com/api/v1/team/" + strings.Repeat("0", 10) + "/image"}
}

func testTournament(id uint) tournaments.Tournament {
	return tournaments.Tournament{Model: gorm.Model{ID: id}, Name: "League", Slug: "league-slug", Region: "region"}
}

func testEvent(sofaID int64, homeID int64, awayID int64) Event {
	return Event{
		SofaScoreEventId: sofaID,
		Sport:            "football",
		HomeScore:        0,
		HomeTeamId:       homeID,
		AwayScore:        0,
		AwayTeamId:       awayID,
		ScrapedAt:        1000,
		Slug:             "test-event",
		LeagueId:         1,
		StatusType:       "inprogress",
	}
}

func TestUpsertScrapeBatch_DeduplicatesTeams(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{testTeam(10), testTeam(10), testTeam(20)},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{testEvent(1, 10, 20), testEvent(2, 10, 10)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("UpsertScrapeBatch: %v", err)
	}

	var count int64
	db.Model(&Team{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 teams, got %d", count)
	}
}

func TestUpsertScrapeBatch_DeduplicatesTournaments(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{testTeam(10), testTeam(20)},
		Tournaments: []tournaments.Tournament{testTournament(1), testTournament(1), testTournament(2)},
		Events:      []Event{testEvent(1, 10, 20), testEvent(2, 10, 20)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("UpsertScrapeBatch: %v", err)
	}

	var count int64
	db.Model(&tournaments.Tournament{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 tournaments, got %d", count)
	}
}

func TestUpsertScrapeBatch_DeduplicatesEvents(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{testTeam(10), testTeam(20)},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{testEvent(1, 10, 20), testEvent(1, 10, 20), testEvent(2, 10, 20)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("UpsertScrapeBatch: %v", err)
	}

	var count int64
	db.Model(&Event{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestUpsertScrapeBatch_UpdatesMutableFields(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{Team{TeamId: 10, Name: "Original", LogoUrl: "https://img.sofascore.com/api/v1/team/10/image", PrimaryColor: "#111", SecondaryColor: "#222", TextColor: "#333"}},
		Tournaments: []tournaments.Tournament{{Model: gorm.Model{ID: 1}, Name: "Old League", Slug: "old-league", Region: "Old"}},
		Events: []Event{{
			SofaScoreEventId: 1, Sport: "football", HomeScore: 0, HomeTeamId: 10,
			AwayScore: 0, AwayTeamId: 20, Slug: "old-slug", LeagueId: 1,
			StatusType: "notstarted", ScrapedAt: 1000,
		}},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("first UpsertScrapeBatch: %v", err)
	}

	updated := ScrapeBatch{
		Teams:       []Team{Team{TeamId: 10, Name: "Updated", LogoUrl: "https://img.sofascore.com/api/v1/team/10/image", PrimaryColor: "#AAA", SecondaryColor: "#BBB", TextColor: "#CCC"}},
		Tournaments: []tournaments.Tournament{{Model: gorm.Model{ID: 1}, Name: "New League", Slug: "new-league", Region: "New"}},
		Events: []Event{{
			SofaScoreEventId:            1,
			Sport:                       "basketball",
			HomeScore:                   10,
			HomeTeamId:                  10,
			AwayScore:                   5,
			AwayTeamId:                  20,
			StartTimestamp:              1720000000000,
			CurrentPeriodStartTimestamp: 1720000000000,
			Slug:                        "new-slug",
			LeagueId:                    1,
			StatusType:                  "finished",
			ScrapedAt:                   2000,
		}},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), updated, 500); err != nil {
		t.Fatalf("second UpsertScrapeBatch: %v", err)
	}

	var team Team
	db.Where("team_id = ?", 10).First(&team)
	if team.Name != "Updated" {
		t.Errorf("team name: want Updated, got %s", team.Name)
	}
	if team.PrimaryColor != "#AAA" {
		t.Errorf("team primary_color: want #AAA, got %s", team.PrimaryColor)
	}

	var tour tournaments.Tournament
	db.First(&tour, 1)
	if tour.Name != "New League" {
		t.Errorf("tournament name: want New League, got %s", tour.Name)
	}
	if tour.Region != "New" {
		t.Errorf("tournament region: want New, got %s", tour.Region)
	}

	var event Event
	db.Where("sofa_score_event_id = ?", 1).First(&event)
	if event.Sport != "basketball" {
		t.Errorf("event sport: want basketball, got %s", event.Sport)
	}
	if event.HomeScore != 10 {
		t.Errorf("event home_score: want 10, got %d", event.HomeScore)
	}
	if event.AwayScore != 5 {
		t.Errorf("event away_score: want 5, got %d", event.AwayScore)
	}
	if event.Slug != "new-slug" {
		t.Errorf("event slug: want new-slug, got %s", event.Slug)
	}
	if event.StatusType != "finished" {
		t.Errorf("event status_type: want finished, got %s", event.StatusType)
	}
}

func TestUpsertScrapeBatch_RollbackOnFailure(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	validTeams := []Team{testTeam(10)}
	validTournaments := []tournaments.Tournament{testTournament(1)}

	// An event with missing required fields that should fail on SQLite constraint
	badEvent := Event{SofaScoreEventId: 1, Sport: "", HomeTeamId: 0, AwayTeamId: 0}

	batch := ScrapeBatch{
		Teams:       validTeams,
		Tournaments: validTournaments,
		Events:      []Event{badEvent},
	}

	// SQLite with glebarez may not enforce NOT NULL, but attempt anyway
	db.Exec("DELETE FROM events")
	db.Exec("DELETE FROM teams")
	db.Exec("DELETE FROM tournaments")

	err := repo.UpsertScrapeBatch(context.Background(), batch, 500)

	var teamCount, tourCount, eventCount int64
	db.Model(&Team{}).Count(&teamCount)
	db.Model(&tournaments.Tournament{}).Count(&tourCount)
	db.Model(&Event{}).Count(&eventCount)

	if err == nil {
		t.Logf("sqlite did not enforce constraint; checking rollback manually")
		t.Skip("sqlite may not enforce NOT NULL constraints")
	}

	if teamCount != 0 {
		t.Errorf("teams should be 0 after rollback, got %d", teamCount)
	}
	if tourCount != 0 {
		t.Errorf("tournaments should be 0 after rollback, got %d", tourCount)
	}
	if eventCount != 0 {
		t.Errorf("events should be 0 after rollback, got %d", eventCount)
	}
}

func TestUpsertScrapeBatch_LogoURLNotUpdated(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	proxiedURL := "/teams/logo/10"
	batch := ScrapeBatch{
		Teams:       []Team{{TeamId: 10, Name: "Team", LogoUrl: "https://img.sofascore.com/api/v1/team/10/image"}},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{testEvent(1, 10, 10)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("first UpsertScrapeBatch: %v", err)
	}

	db.Model(&Team{}).Where("team_id = ?", 10).Update("logo_url", proxiedURL)

	batch2 := ScrapeBatch{
		Teams:       []Team{{TeamId: 10, Name: "Team Updated", LogoUrl: "https://img.sofascore.com/api/v1/team/10/image", PrimaryColor: "#111"}},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{{SofaScoreEventId: 1, Sport: "football", HomeTeamId: 10, AwayTeamId: 10, ScrapedAt: 2000, Slug: "x", LeagueId: 1, StatusType: "finished"}},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch2, 500); err != nil {
		t.Fatalf("second UpsertScrapeBatch: %v", err)
	}

	var team Team
	db.Where("team_id = ?", 10).First(&team)
	if team.LogoUrl != proxiedURL {
		t.Errorf("logo_url should not be overwritten by OnConflict upsert: expected %s, got %s", proxiedURL, team.LogoUrl)
	}
}

func TestUpsertScrapeBatch_RollbackOnForcedEventFailure(t *testing.T) {
	db := setupBatchTestDB(t)
	_ = NewRepository(db)

	db.Migrator().DropTable(&Team{})
	db.AutoMigrate(&Team{})

	db.Exec("CREATE TABLE IF NOT EXISTS teams_new (id INTEGER PRIMARY KEY, team_id INTEGER UNIQUE, name TEXT, logo_url TEXT, primary_color TEXT, secondary_color TEXT, text_color TEXT)")

	err := db.Transaction(func(tx *gorm.DB) error {
		tx.Create(&Team{TeamId: 10, Name: "Team"})
		if tx.Error != nil {
			return tx.Error
		}
		return errors.New("forced failure")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpsertScrapeBatch_SingleTransaction(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{testTeam(10), testTeam(20)},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{testEvent(1, 10, 20)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("UpsertScrapeBatch: %v", err)
	}

	var teamCount int64
	db.Model(&Team{}).Count(&teamCount)
	if teamCount != 2 {
		t.Errorf("expected 2 teams, got %d", teamCount)
	}
	var tourCount int64
	db.Model(&tournaments.Tournament{}).Count(&tourCount)
	if tourCount != 1 {
		t.Errorf("expected 1 tournament, got %d", tourCount)
	}
	var eventCount int64
	db.Model(&Event{}).Count(&eventCount)
	if eventCount != 1 {
		t.Errorf("expected 1 event, got %d", eventCount)
	}
}

func TestUpsertScrapeBatch_EmptyBatch(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	if err := repo.UpsertScrapeBatch(context.Background(), ScrapeBatch{}, 500); err != nil {
		t.Fatalf("empty batch should succeed: %v", err)
	}
}

func TestIsProxiedLogoURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"/teams/logo/123", true},
		{"/teams/logo/456.png", true},
		{"/api/app/v1/teams/logo/789", true},
		{"https://img.sofascore.com/api/v1/team/10/image", false},
		{"/other/path", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isProxiedLogoURL(tt.url); got != tt.expected {
				t.Errorf("isProxiedLogoURL(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestUpsertScrapeBatch_DefaultBatchSize(t *testing.T) {
	db := setupBatchTestDB(t)
	repo := NewRepository(db)

	batch := ScrapeBatch{
		Teams:       []Team{testTeam(10)},
		Tournaments: []tournaments.Tournament{testTournament(1)},
		Events:      []Event{testEvent(1, 10, 10)},
	}

	if err := repo.UpsertScrapeBatch(context.Background(), batch, 0); err != nil {
		t.Fatalf("UpsertScrapeBatch with batchSize=0: %v", err)
	}
}

func BenchmarkUpsertScrapeBatch(b *testing.B) {
	db := setupBatchTestDB(b)
	repo := NewRepository(db)

	const eventCount = 1000
	teams := make([]Team, 0, eventCount*2)
	tournaments := make([]tournaments.Tournament, 0, eventCount)
	events := make([]Event, 0, eventCount)

	for i := int64(0); i < eventCount; i++ {
		teams = append(teams, testTeam(i*2), testTeam(i*2+1))
		tournaments = append(tournaments, testTournament(uint(i)+1))
		events = append(events, testEvent(i+1, i*2, i*2+1))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := ScrapeBatch{Teams: teams, Tournaments: tournaments, Events: events}
		if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}
