package reporting

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

type EventStats struct {
	SofaScoreEventId int64
	ViewCount        int64
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveCrash(report CrashReport) error {
	return r.db.Create(&report).Error
}

// GetTopEvents returns the most-viewed events by content, ignoring
// non-numeric content rows. The ctx is propagated to the underlying
// query so a Gin request cancellation or a caller-supplied deadline
// actually halts the SQL round-trip.
//
// The MariaDB/MySQL branch uses `content REGEXP '^[0-9]+$'` which
// cannot use a standard B-tree index on the text `content` column.
// Indexing this query requires a functional index on a generated
// typed column (e.g. `CAST(content AS UNSIGNED)`); tracked in a
// follow-up migration issue, out of scope here.
func (r *Repository) GetTopEvents(ctx context.Context, limit int) ([]EventStats, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var stats []EventStats
	var result *gorm.DB

	if strings.Contains(r.db.Dialector.Name(), "sqlite") {
		result = r.db.WithContext(ctx).Raw(`
			SELECT CAST(content AS INTEGER) AS sofa_score_event_id,
			       COUNT(*) AS view_count
			FROM playback_logs
			WHERE content NOT GLOB '*[^0-9]*' AND content != ''
			GROUP BY content
			ORDER BY view_count DESC, sofa_score_event_id ASC
			LIMIT ?
		`, limit).Scan(&stats)
	} else {
		result = r.db.WithContext(ctx).Raw(`
			SELECT CAST(content AS UNSIGNED) AS sofa_score_event_id,
			       COUNT(*) AS view_count
			FROM playback_logs
			WHERE content REGEXP '^[0-9]+$'
			GROUP BY content
			ORDER BY view_count DESC, sofa_score_event_id ASC
			LIMIT ?
		`, limit).Scan(&stats)
	}

	return stats, result.Error
}
