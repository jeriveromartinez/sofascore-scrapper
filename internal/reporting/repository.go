package reporting

import "gorm.io/gorm"

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

func (r *Repository) GetTopEvents(limit int) ([]EventStats, error) {
	var stats []EventStats
	result := r.db.Table("playback_logs").
		Select("content, count(*) as view_count").
		Group("content").
		Order("view_count desc").
		Limit(limit).
		Scan(&stats)
	return stats, result.Error
}
