package playback

import "testing"

// The nightly reporting rollup filters playback_logs on ended_at, so the
// column needs its own index to avoid a full table scan.
func TestPlaybackLogSchemaIndexesEndedAt(t *testing.T) {
	db := setupTestDB(t)

	indexes, err := db.Migrator().GetIndexes(&PlaybackLog{})
	if err != nil {
		t.Fatalf("get indexes: %v", err)
	}
	for _, idx := range indexes {
		if idx.Name() != "idx_playback_logs_ended_at" {
			continue
		}
		cols := idx.Columns()
		if len(cols) != 1 || cols[0] != "ended_at" {
			t.Fatalf("idx_playback_logs_ended_at columns = %v, want [ended_at]", cols)
		}
		return
	}
	t.Fatal("idx_playback_logs_ended_at is not declared on playback_logs")
}
