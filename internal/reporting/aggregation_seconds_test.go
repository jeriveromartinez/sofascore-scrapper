package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/playback"
)

// TestGenerateDaily_TruncatesMillisecondsToWholeSeconds locks the
// millisecond-to-second mapping: durations are summed in milliseconds
// and truncated to whole seconds when the daily row is built.
func TestGenerateDaily_TruncatesMillisecondsToWholeSeconds(t *testing.T) {
	db := newWindowTestDB(t)
	now := time.Now()
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).
		AddDate(0, 0, -1).Add(time.Hour).UnixMilli()

	logs := []playback.PlaybackLog{
		{DeviceID: 1, Content: "half", StartedAt: base + 1000, EndedAt: base + 31500},
		{DeviceID: 2, Content: "four-five", StartedAt: base + 5000, EndedAt: base + 9500},
	}
	for i := range logs {
		if err := db.Create(&logs[i]).Error; err != nil {
			t.Fatalf("seed playback log %q: %v", logs[i].Content, err)
		}
	}

	if err := NewAggregationRepository(db).GenerateDaily(context.Background()); err != nil {
		t.Fatalf("GenerateDaily failed: %v", err)
	}

	want := map[string]int{"half": 30, "four-five": 4}
	var stats []ContentStat
	if err := db.Where("period_type = ?", PeriodTypeDay).Find(&stats).Error; err != nil {
		t.Fatalf("query daily stats: %v", err)
	}
	if len(stats) != len(want) {
		t.Fatalf("daily rows: want %d, got %d", len(want), len(stats))
	}
	for _, stat := range stats {
		if stat.Seconds != want[stat.ContentHash] {
			t.Errorf("%s seconds = %d, want %d", stat.ContentHash, stat.Seconds, want[stat.ContentHash])
		}
		if stat.Views != 1 {
			t.Errorf("%s views = %d, want 1", stat.ContentHash, stat.Views)
		}
	}
}
