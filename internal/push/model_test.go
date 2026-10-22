package push

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestSchemaScheduledPushTimer_DueTimersIndex pins the index that
// serves ListDueTimers (dispatched_at IS NULL AND fire_at <= ?
// ORDER BY fire_at), which runs on every scheduler tick.
func TestSchemaScheduledPushTimer_DueTimersIndex(t *testing.T) {
	s, err := schema.Parse(&ScheduledPushTimer{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	var idx *schema.Index
	for _, candidate := range s.ParseIndexes() {
		if candidate.Name == "idx_spt_dispatched_fire" {
			idx = candidate
		}
	}
	if idx == nil {
		t.Fatal("expected index idx_spt_dispatched_fire on scheduled_push_timers")
	}
	want := []string{"dispatched_at", "fire_at"}
	if len(idx.Fields) != len(want) {
		t.Fatalf("idx_spt_dispatched_fire has %d columns, want %v", len(idx.Fields), want)
	}
	for i, col := range want {
		if got := idx.Fields[i].DBName; got != col {
			t.Errorf("idx_spt_dispatched_fire column %d = %q, want %q", i, got, col)
		}
	}
}
