package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/playback"
)

// TestGenerateDaily_HonoursCancelledContext asserts the rollup runs on
// the caller's ctx: a cancelled scheduler job must not start the
// transaction, and the source rows must stay untouched.
func TestGenerateDaily_HonoursCancelledContext(t *testing.T) {
	db := newWindowTestDB(t)
	repo := NewAggregationRepository(db)

	yesterday := time.Now().AddDate(0, 0, -1)
	insertWindowLog(t, db, "cancelled", yesterday.UnixMilli())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.GenerateDaily(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateDaily(cancelled ctx) error = %v, want context.Canceled", err)
	}

	var remaining int64
	if err := db.Model(&playback.PlaybackLog{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count playback logs: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("playback logs = %d, want 1 (a cancelled rollup must not delete them)", remaining)
	}
}

func TestGenerateMonthly_HonoursCancelledContext(t *testing.T) {
	db := newWindowTestDB(t)
	repo := NewAggregationRepository(db)

	begin, _ := monthlyWindow(time.Now(), time.Local)
	if err := db.Create(&ContentStat{
		ContentHash: "cancelled",
		PeriodType:  PeriodTypeDay,
		PeriodStart: begin,
		Seconds:     10,
		Views:       1,
	}).Error; err != nil {
		t.Fatalf("seed day stat: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.GenerateMonthly(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateMonthly(cancelled ctx) error = %v, want context.Canceled", err)
	}

	var dayRows int64
	if err := db.Model(&ContentStat{}).Where("period_type = ?", PeriodTypeDay).Count(&dayRows).Error; err != nil {
		t.Fatalf("count day stats: %v", err)
	}
	if dayRows != 1 {
		t.Fatalf("day stats = %d, want 1 (a cancelled rollup must not delete them)", dayRows)
	}
}
