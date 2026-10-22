package reporting

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

const periodIndexName = "idx_content_stats_period"

// TestContentStatHasPeriodIndex guards the monthly rollup, which filters
// on period_type and a period_start range without a content_hash: the
// unique (content_hash, period_type, period_start) index cannot serve it.
func TestContentStatHasPeriodIndex(t *testing.T) {
	s, err := schema.Parse(&ContentStat{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}

	var idx *schema.Index
	for _, candidate := range s.ParseIndexes() {
		if candidate.Name == periodIndexName {
			idx = candidate
		}
	}
	if idx == nil {
		t.Fatalf("index %s not declared on ContentStat", periodIndexName)
	}
	var columns []string
	for _, f := range idx.Fields {
		columns = append(columns, f.DBName)
	}
	if len(columns) != 2 || columns[0] != "period_type" || columns[1] != "period_start" {
		t.Fatalf("index %s columns = %v, want [period_type period_start]", periodIndexName, columns)
	}
	if idx.Class == "UNIQUE" {
		t.Fatalf("index %s must not be unique", periodIndexName)
	}

	db := newWindowTestDB(t)
	if !db.Migrator().HasIndex(&ContentStat{}, periodIndexName) {
		t.Fatalf("index %s missing after AutoMigrate", periodIndexName)
	}
}
