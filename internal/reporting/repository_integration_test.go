//go:build integration

package reporting

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"gorm.io/gorm"
)

func setupRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&playback.PlaybackLog{}, &ContentStat{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func createPlaybackLog(db *gorm.DB, content string, deviceID uint) {
	db.Create(&playback.PlaybackLog{DeviceID: deviceID, Content: content, StartedAt: 1000, EndedAt: 2000})
}

func TestGetTopEvents_NumericContentOnly(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	createPlaybackLog(db, "12345", 1)
	createPlaybackLog(db, "12345", 2)
	createPlaybackLog(db, "not-a-number", 3)
	createPlaybackLog(db, "67890", 4)

	stats, err := repo.GetTopEvents(100)
	if err != nil {
		t.Fatalf("GetTopEvents failed: %v", err)
	}

	for _, s := range stats {
		if s.SofaScoreEventId == 0 {
			t.Errorf("unexpected zero SofaScoreEventId, non-numeric content should be excluded")
		}
	}

	found12345 := false
	found67890 := false
	for _, s := range stats {
		if s.SofaScoreEventId == 12345 {
			found12345 = true
			if s.ViewCount != 2 {
				t.Errorf("event 12345: want view_count=2, got %d", s.ViewCount)
			}
		}
		if s.SofaScoreEventId == 67890 {
			found67890 = true
			if s.ViewCount != 1 {
				t.Errorf("event 67890: want view_count=1, got %d", s.ViewCount)
			}
		}
	}
	if !found12345 {
		t.Error("expected event 12345 in results")
	}
	if !found67890 {
		t.Error("expected event 67890 in results")
	}
}

func TestGetTopEvents_DeterministicOrdering(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	createPlaybackLog(db, "200", 1)
	createPlaybackLog(db, "200", 2)
	createPlaybackLog(db, "100", 3)
	createPlaybackLog(db, "100", 4)
	createPlaybackLog(db, "300", 5)

	stats, err := repo.GetTopEvents(100)
	if err != nil {
		t.Fatalf("GetTopEvents failed: %v", err)
	}

	if len(stats) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(stats))
	}

	if stats[0].SofaScoreEventId != 100 {
		t.Errorf("first event: want 100 (2 views), got %d", stats[0].SofaScoreEventId)
	}
	if stats[0].ViewCount != 2 {
		t.Errorf("first event views: want 2, got %d", stats[0].ViewCount)
	}
	if stats[1].SofaScoreEventId != 200 {
		t.Errorf("second event: want 200 (2 views), got %d (tie-break by sofa_score_event_id ASC)", stats[1].SofaScoreEventId)
	}
	if stats[2].SofaScoreEventId != 300 {
		t.Errorf("third event: want 300 (1 view), got %d", stats[2].SofaScoreEventId)
	}
}

func TestGetTopEvents_CapAt100(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	for i := 1; i <= 150; i++ {
		createPlaybackLog(db, fmt.Sprintf("%d", i), uint(i))
	}

	stats, err := repo.GetTopEvents(200)
	if err != nil {
		t.Fatalf("GetTopEvents failed: %v", err)
	}

	if len(stats) > 100 {
		t.Errorf("expected at most 100 results, got %d", len(stats))
	}
}

func TestGetTopEvents_ZeroLimitDefaultsTo100(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	for i := 1; i <= 120; i++ {
		createPlaybackLog(db, fmt.Sprintf("%d", i), uint(i))
	}

	stats, err := repo.GetTopEvents(0)
	if err != nil {
		t.Fatalf("GetTopEvents failed: %v", err)
	}

	if len(stats) > 100 {
		t.Errorf("zero limit: expected at most 100 results, got %d", len(stats))
	}
}

func TestGetTopEvents_ReturnsDBError(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	createPlaybackLog(db, "999", 1)

	sqlDB, _ := db.DB()
	sqlDB.Close()

	_, err := repo.GetTopEvents(10)
	if err == nil {
		t.Error("expected error when DB is closed")
	}
}
