//go:build integration

package reporting

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"gorm.io/gorm"
)

func setupAggTestDB(t *testing.T) *gorm.DB {
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

func createEndedPlaybackLog(db *gorm.DB, content string, deviceID uint, endedAt int64) {
	db.Create(&playback.PlaybackLog{
		DeviceID:  deviceID,
		Content:   content,
		StartedAt: endedAt - 30000,
		EndedAt:   endedAt,
	})
}

func TestGenerateDaily_CreatesContentStats(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	createEndedPlaybackLog(db, "channel-a", 1, yesterday.UnixMilli())
	createEndedPlaybackLog(db, "channel-b", 2, yesterday.UnixMilli())
	createEndedPlaybackLog(db, "channel-a", 3, yesterday.UnixMilli()+1000)
	createEndedPlaybackLog(db, "channel-a", 4, now.UnixMilli())

	err := repo.GenerateDaily()
	if err != nil {
		t.Fatalf("GenerateDaily failed: %v", err)
	}

	var stats []ContentStat
	db.Where("period_type = ?", PeriodTypeDay).Find(&stats)
	if len(stats) < 2 {
		t.Fatalf("expected at least 2 daily stats, got %d", len(stats))
	}

	totalViews := 0
	for _, s := range stats {
		totalViews += s.Views
	}
	if totalViews != 3 {
		t.Errorf("expected 3 views processed, got %d", totalViews)
	}
}

func TestGenerateDaily_DeletesProcessedLogs(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	createEndedPlaybackLog(db, "ch", 1, yesterday.UnixMilli())
	createEndedPlaybackLog(db, "ch", 2, now.UnixMilli())

	err := repo.GenerateDaily()
	if err != nil {
		t.Fatalf("GenerateDaily failed: %v", err)
	}

	var count int64
	db.Model(&playback.PlaybackLog{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining log, got %d", count)
	}
}

func TestGenerateDaily_MillisecondDuration(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	begin := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Add(-24 * time.Hour)
	yesterdayMs := begin.UnixMilli()

	db.Create(&playback.PlaybackLog{
		DeviceID:  1,
		Content:   "ch",
		StartedAt: yesterdayMs + 1000,
		EndedAt:   yesterdayMs + 1000 + 60000,
	})

	err := repo.GenerateDaily()
	if err != nil {
		t.Fatalf("GenerateDaily failed: %v", err)
	}

	var stat ContentStat
	db.Where("period_type = ? AND content_hash = ?", PeriodTypeDay, "ch").First(&stat)
	if stat.Seconds != 60 {
		t.Errorf("want 60 seconds time_played, got %d (verify ms to s conversion)", stat.Seconds)
	}
}

func TestGenerateDaily_UpsertsExistingPeriodIdempotently(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	begin := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Add(-24 * time.Hour)
	existing := ContentStat{
		ContentHash: "late-channel",
		PeriodType:  PeriodTypeDay,
		PeriodStart: begin,
		Seconds:     10,
		Views:       1,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed existing daily stat: %v", err)
	}
	createEndedPlaybackLog(db, "late-channel", 2, begin.Add(time.Hour).UnixMilli())

	if err := repo.GenerateDaily(); err != nil {
		t.Fatalf("GenerateDaily with an existing period failed: %v", err)
	}

	var stats []ContentStat
	if err := db.Where("content_hash = ? AND period_type = ? AND period_start = ?", "late-channel", PeriodTypeDay, begin).Find(&stats).Error; err != nil {
		t.Fatalf("query daily stat: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("daily rows: want 1, got %d", len(stats))
	}
	if stats[0].Seconds != 30 || stats[0].Views != 1 {
		t.Fatalf("updated daily stat: want 30 seconds/1 view, got %d seconds/%d views", stats[0].Seconds, stats[0].Views)
	}

	if err := repo.GenerateDaily(); err != nil {
		t.Fatalf("GenerateDaily retry failed: %v", err)
	}
	var afterRetry ContentStat
	if err := db.First(&afterRetry, stats[0].ID).Error; err != nil {
		t.Fatalf("query daily stat after retry: %v", err)
	}
	if afterRetry.Seconds != stats[0].Seconds || afterRetry.Views != stats[0].Views {
		t.Fatalf("retry changed daily stat from %d/%d to %d/%d", stats[0].Seconds, stats[0].Views, afterRetry.Seconds, afterRetry.Views)
	}
}

func TestGenerateMonthly_UsesPeriodStart(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	monthBegin := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)

	day1 := ContentStat{ContentHash: "ch1", PeriodType: PeriodTypeDay, PeriodStart: monthBegin, Seconds: 100, Views: 10}
	day2 := ContentStat{ContentHash: "ch1", PeriodType: PeriodTypeDay, PeriodStart: monthBegin.AddDate(0, 0, 15), Seconds: 200, Views: 20}
	db.Create(&day1)
	db.Create(&day2)

	dayBefore := ContentStat{ContentHash: "ch1", PeriodType: PeriodTypeDay, PeriodStart: monthBegin.Add(-24 * time.Hour), Seconds: 50, Views: 5}
	db.Create(&dayBefore)

	dayAfter := ContentStat{ContentHash: "ch1", PeriodType: PeriodTypeDay, PeriodStart: monthBegin.AddDate(0, 1, 0), Seconds: 300, Views: 30}
	db.Create(&dayAfter)

	err := repo.GenerateMonthly()
	if err != nil {
		t.Fatalf("GenerateMonthly failed: %v", err)
	}

	var monthStat ContentStat
	result := db.Where("period_type = ? AND content_hash = ?", PeriodTypeMonth, "ch1").First(&monthStat)
	if result.Error != nil {
		t.Fatalf("monthly stat not found: %v", result.Error)
	}
	if monthStat.Seconds != 300 {
		t.Errorf("want 300 seconds (100+200), got %d", monthStat.Seconds)
	}
	if monthStat.Views != 30 {
		t.Errorf("want 30 views (10+20), got %d", monthStat.Views)
	}
}

func TestGenerateMonthly_HalfOpenRange(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	monthBegin := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	nextMonthBegin := monthBegin.AddDate(0, 1, 0)

	atMonthStart := ContentStat{ContentHash: "ch", PeriodType: PeriodTypeDay, PeriodStart: monthBegin, Seconds: 10, Views: 1}
	db.Create(&atMonthStart)

	atNextMonthStart := ContentStat{ContentHash: "ch", PeriodType: PeriodTypeDay, PeriodStart: nextMonthBegin, Seconds: 999, Views: 99}
	db.Create(&atNextMonthStart)

	err := repo.GenerateMonthly()
	if err != nil {
		t.Fatalf("GenerateMonthly failed: %v", err)
	}

	var monthStat ContentStat
	db.Where("period_type = ? AND content_hash = ?", PeriodTypeMonth, "ch").First(&monthStat)
	if monthStat.Seconds != 10 {
		t.Errorf("half-open: want 10 seconds (period_start >= begin AND period_start < end), got %d", monthStat.Seconds)
	}
}

func TestGenerateMonthly_Idempotent(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	monthBegin := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)

	db.Create(&ContentStat{ContentHash: "ch", PeriodType: PeriodTypeDay, PeriodStart: monthBegin, Seconds: 50, Views: 5})

	if err := repo.GenerateMonthly(); err != nil {
		t.Fatalf("first GenerateMonthly call failed: %v", err)
	}

	var firstResult ContentStat
	db.Where("period_type = ? AND content_hash = ?", PeriodTypeMonth, "ch").First(&firstResult)

	if err := repo.GenerateMonthly(); err != nil {
		t.Fatalf("second GenerateMonthly (no new data) failed: %v", err)
	}

	var secondResult ContentStat
	db.Where("period_type = ? AND content_hash = ?", PeriodTypeMonth, "ch").First(&secondResult)

	if secondResult.Seconds != firstResult.Seconds {
		t.Errorf("idempotent: seconds changed from %d to %d on re-run", firstResult.Seconds, secondResult.Seconds)
	}
	if secondResult.Views != firstResult.Views {
		t.Errorf("idempotent: views changed from %d to %d on re-run", firstResult.Views, secondResult.Views)
	}
}

func TestGenerateMonthly_DeletesDailyRows(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	now := time.Now()
	monthBegin := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)

	for i := 0; i < 5; i++ {
		db.Create(&ContentStat{
			ContentHash: fmt.Sprintf("ch%d", i),
			PeriodType:  PeriodTypeDay,
			PeriodStart: monthBegin.AddDate(0, 0, i),
			Seconds:     10,
			Views:       1,
		})
	}

	err := repo.GenerateMonthly()
	if err != nil {
		t.Fatalf("GenerateMonthly failed: %v", err)
	}

	var dailyCount int64
	db.Model(&ContentStat{}).Where("period_type = ?", PeriodTypeDay).Count(&dailyCount)
	if dailyCount != 0 {
		t.Errorf("expected 0 daily rows after monthly aggregation, got %d", dailyCount)
	}

	var monthlyCount int64
	db.Model(&ContentStat{}).Where("period_type = ?", PeriodTypeMonth).Count(&monthlyCount)
	if monthlyCount != 5 {
		t.Errorf("expected 5 monthly rows, got %d", monthlyCount)
	}
}

func TestGenerateMonthly_EmptyPeriodDoesNotFail(t *testing.T) {
	db := setupAggTestDB(t)
	repo := NewAggregationRepository(db)

	err := repo.GenerateDaily()
	if err != nil {
		t.Errorf("GenerateDaily with no logs should not error: %v", err)
	}

	err = repo.GenerateMonthly()
	if err != nil {
		t.Errorf("GenerateMonthly with no daily data should not error: %v", err)
	}
}
