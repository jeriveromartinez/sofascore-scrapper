package events

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

var currentEventsDBCounter atomic.Int64

func setupCurrentEventsDB(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:events-current-%d?mode=memory&cache=shared", currentEventsDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), db
}

func mustCreate(t *testing.T, db *gorm.DB, value any) {
	t.Helper()
	if err := db.Create(value).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func seedCurrentEvent(t *testing.T, db *gorm.DB, externalID, status string, startTs int64, leagueID uint) uint {
	t.Helper()
	event := Event{
		ExternalMatchId: externalID,
		Sport:           "football",
		StatusType:      status,
		StartTimestamp:  startTs,
		LeagueId:        leagueID,
		// The scraper stamps every upsert, so fixtures must too:
		// an unwritten scraped_at would look stale to the live filter.
		ScrapedAt: time.Now().Unix(),
	}
	mustCreate(t, db, &event)
	return event.ID
}

func eventIDs(events []Event) []uint {
	ids := make([]uint, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}

func TestGetCurrentAndUpcoming_DeterministicOrdering(t *testing.T) {
	repo, db := setupCurrentEventsDB(t)

	league := tournaments.Tournament{Name: "League", Slug: "league"}
	mustCreate(t, db, &league)

	now := time.Now().UnixMilli()
	older := seedCurrentEvent(t, db, "live-old", "inprogress", now-7200_000, league.ID)
	liveTieLow := seedCurrentEvent(t, db, "live-tie-low", "inprogress", now-3600_000, league.ID)
	liveTieHigh := seedCurrentEvent(t, db, "live-tie-high", "inprogress", now-3600_000, league.ID)
	nextFirst := seedCurrentEvent(t, db, "next-first", "notstarted", now+3600_000, league.ID)
	nextTieLow := seedCurrentEvent(t, db, "next-tie-low", "notstarted", now+7200_000, league.ID)
	nextTieHigh := seedCurrentEvent(t, db, "next-tie-high", "notstarted", now+7200_000, league.ID)

	mustCreate(t, db, &tournaments.GlobalTournamentConfig{TournamentID: league.ID})

	want := []uint{liveTieHigh, liveTieLow, older, nextFirst, nextTieLow, nextTieHigh}
	for attempt := 1; attempt <= 2; attempt++ {
		got, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
		if err != nil {
			t.Fatalf("attempt %d: GetCurrentAndUpcoming: %v", attempt, err)
		}
		if ids := eventIDs(got); !slices.Equal(ids, want) {
			t.Fatalf("attempt %d: got event ids %v, want %v", attempt, ids, want)
		}
	}
}

func TestGetCurrentAndUpcoming_ExcludesSoftDeletedLeague(t *testing.T) {
	repo, db := setupCurrentEventsDB(t)

	active := tournaments.Tournament{Name: "Active", Slug: "active"}
	deleted := tournaments.Tournament{
		Model: gorm.Model{DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		Name:  "Deleted",
		Slug:  "deleted",
	}
	mustCreate(t, db, &active)
	mustCreate(t, db, &deleted)

	now := time.Now().UnixMilli()
	seedCurrentEvent(t, db, "live-active", "inprogress", now-3600_000, active.ID)
	seedCurrentEvent(t, db, "live-deleted", "inprogress", now-1800_000, deleted.ID)
	mustCreate(t, db, &tournaments.GlobalTournamentConfig{TournamentID: active.ID})
	mustCreate(t, db, &tournaments.GlobalTournamentConfig{TournamentID: deleted.ID})

	got, err := repo.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming: %v", err)
	}
	if len(got) != 1 || got[0].ExternalMatchId != "live-active" {
		t.Fatalf("got %d events, want only live-active: %+v", len(got), got)
	}
	if got[0].League == nil {
		t.Fatalf("live-active event returned without a preloaded league")
	}
}

func TestResolveTournamentIDs_DeterministicTruncation(t *testing.T) {
	repo, db := setupCurrentEventsDB(t)

	seeded := make([]tournaments.Tournament, 0, maxTournamentIDs+50)
	for id := uint(1); id <= maxTournamentIDs+50; id++ {
		seeded = append(seeded, tournaments.Tournament{Model: gorm.Model{ID: id}, Name: "League " + strconv.Itoa(int(id)), Slug: "league-" + strconv.Itoa(int(id))})
	}
	if err := db.CreateInBatches(seeded, 100).Error; err != nil {
		t.Fatalf("seed leagues: %v", err)
	}

	want := make([]uint, maxTournamentIDs)
	for i := range want {
		want[i] = uint(i + 1)
	}

	t.Run("device assignments", func(t *testing.T) {
		const total = maxTournamentIDs + 50
		for i := 0; i < total; i++ {
			// A coprime stride inserts the ids in shuffled order.
			tournamentID := uint((i*37)%total + 1)
			mustCreate(t, db, &tournaments.DeviceTournament{DeviceID: 42, TournamentID: tournamentID})
		}
		for attempt := 1; attempt <= 2; attempt++ {
			got, err := repo.ResolveTournamentIDs(context.Background(), 42)
			if err != nil {
				t.Fatalf("attempt %d: ResolveTournamentIDs: %v", attempt, err)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("attempt %d: got %v, want the %d lowest ids", attempt, got, maxTournamentIDs)
			}
		}
	})

	t.Run("global config fallback", func(t *testing.T) {
		const total = maxTournamentIDs + 50
		for i := 0; i < total; i++ {
			tournamentID := uint((i*37)%total + 1)
			mustCreate(t, db, &tournaments.GlobalTournamentConfig{TournamentID: tournamentID})
		}
		for attempt := 1; attempt <= 2; attempt++ {
			got, err := repo.ResolveTournamentIDs(context.Background(), 7)
			if err != nil {
				t.Fatalf("attempt %d: ResolveTournamentIDs: %v", attempt, err)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("attempt %d: got %v, want the %d lowest ids", attempt, got, maxTournamentIDs)
			}
		}
	})
}

func TestTruncateTournamentIDsWarnsOncePerDevice(t *testing.T) {
	records := captureSlog(t)
	repo, _ := setupCurrentEventsDB(t)

	ids := make([]uint, maxTournamentIDs+5)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if got := repo.truncateTournamentIDs(77, ids); len(got) != maxTournamentIDs {
			t.Fatalf("attempt %d: len = %d, want %d", attempt, len(got), maxTournamentIDs)
		}
	}

	records.mu.Lock()
	output := records.buf.String()
	records.mu.Unlock()
	if n := strings.Count(output, "truncating device tournament assignments"); n != 1 {
		t.Fatalf("truncation warning logged %d times, want 1:\n%s", n, output)
	}
}
