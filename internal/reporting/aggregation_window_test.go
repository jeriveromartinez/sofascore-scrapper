package reporting

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/playback"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newWindowTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reporting-window-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&playback.PlaybackLog{}, &ContentStat{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func insertWindowLog(t *testing.T, db *gorm.DB, content string, endedAt int64) {
	t.Helper()
	if err := db.Create(&playback.PlaybackLog{
		DeviceID:  1,
		Content:   content,
		StartedAt: endedAt - 30_000,
		EndedAt:   endedAt,
	}).Error; err != nil {
		t.Fatalf("seed playback log %q: %v", content, err)
	}
}

// TestGenerateDaily_WindowEndsAtNextLocalMidnight is the regression
// test for the off-by-one in the daily rollup window.
//
// The end bound used to be "next midnight minus one second" compared
// inclusively, so the last second of the day fell outside the window:
// those rows were neither rolled up nor deleted and stayed stranded in
// playback_logs forever.
func TestGenerateDaily_WindowEndsAtNextLocalMidnight(t *testing.T) {
	db := newWindowTestDB(t)
	repo := NewAggregationRepository(db)

	// The window must end at the next local midnight. The expectation
	// is spelled out here instead of taken from the code under test.
	year, month, day := time.Now().In(time.Local).Date()
	end := time.Date(year, month, day, 0, 0, 0, 0, time.Local)

	// The last millisecond of the window belongs to the previous day.
	insertWindowLog(t, db, "last-ms", end.UnixMilli()-1)
	// The end bound itself belongs to the next day.
	insertWindowLog(t, db, "next-day", end.UnixMilli())

	if err := repo.GenerateDaily(); err != nil {
		t.Fatalf("GenerateDaily failed: %v", err)
	}

	var rolledUp ContentStat
	if err := db.Where("content_hash = ? AND period_type = ?", "last-ms", PeriodTypeDay).First(&rolledUp).Error; err != nil {
		t.Fatalf("the last millisecond of the day was not rolled up: %v", err)
	}
	if rolledUp.Views != 1 {
		t.Fatalf("views = %d, want 1", rolledUp.Views)
	}

	var nextDay ContentStat
	err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
		Where("content_hash = ? AND period_type = ?", "next-day", PeriodTypeDay).
		First(&nextDay).Error
	if err == nil {
		t.Fatal("a row at the end bound was rolled up into the previous day")
	}

	var remaining int64
	if err := db.Model(&playback.PlaybackLog{}).Where("content = ?", "next-day").Count(&remaining).Error; err != nil {
		t.Fatalf("count playback logs: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("the row at the end bound was deleted by the cleanup")
	}
}

// TestDailyWindow_UsesCalendarArithmeticAcrossDST is the regression
// test for the DST-unsafe day window.
//
// The window used to start at "today's midnight minus 24 hours". On a
// DST transition day that is not the previous local midnight: the
// window started an hour late, so the first hour of the day was never
// rolled up and the first hour of the next day was rolled up twice.
func TestDailyWindow_UsesCalendarArithmeticAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	cases := []struct {
		name     string
		now      time.Time
		wantLen  time.Duration
		wantFrom time.Time
		wantTo   time.Time
	}{
		{
			// The window covers the previous day, so a rollup run on
			// 2026-11-02 covers 2026-11-01, the day the clocks go back
			// at 02:00: it is 25 hours long.
			name:     "fall back",
			now:      time.Date(2026, 11, 2, 12, 0, 0, 0, loc),
			wantLen:  25 * time.Hour,
			wantFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:   time.Date(2026, 11, 2, 0, 0, 0, 0, loc),
		},
		{
			// A rollup run on 2026-03-09 covers 2026-03-08, the day
			// the clocks go forward at 02:00: it is 23 hours long.
			name:     "spring forward",
			now:      time.Date(2026, 3, 9, 12, 0, 0, 0, loc),
			wantLen:  23 * time.Hour,
			wantFrom: time.Date(2026, 3, 8, 0, 0, 0, 0, loc),
			wantTo:   time.Date(2026, 3, 9, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			begin, end := dailyWindow(tc.now, loc)
			if !begin.Equal(tc.wantFrom) {
				t.Errorf("window start = %s, want %s", begin, tc.wantFrom)
			}
			if !end.Equal(tc.wantTo) {
				t.Errorf("window end = %s, want %s", end, tc.wantTo)
			}
			if got := end.Sub(begin); got != tc.wantLen {
				t.Errorf("window length = %s, want %s", got, tc.wantLen)
			}
		})
	}
}

// TestMonthlyWindow_CoversPreviousCalendarMonth pins the monthly
// window on the previous calendar month, with an exclusive end bound.
func TestMonthlyWindow_CoversPreviousCalendarMonth(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	now := time.Date(2026, 11, 15, 9, 30, 0, 0, loc)
	begin, end := monthlyWindow(now, loc)

	wantBegin := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	if !begin.Equal(wantBegin) {
		t.Errorf("monthly window start = %s, want %s", begin, wantBegin)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("monthly window end = %s, want %s", end, wantEnd)
	}
}
