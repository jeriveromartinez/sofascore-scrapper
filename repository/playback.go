package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

type EventStats struct {
	SofaScoreEventId int64
	ViewCount        int64
}

func LogPlayback(deviceID uint, sofaScoreEventId int64, startedAt int64) (*models.PlaybackLog, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	log := &models.PlaybackLog{DeviceID: deviceID, SofaScoreEventId: sofaScoreEventId, StartedAt: startedAt}
	result := db.Create(log)
	return log, result.Error
}

func UpdatePlaybackEnd(id uint, endedAt int64) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Model(&models.PlaybackLog{}).Where("id = ?", id).Update("ended_at", endedAt).Error
}

func GetTopEvents(limit int) ([]EventStats, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var stats []EventStats
	result := db.Model(&models.PlaybackLog{}).
		Select("sofa_score_event_id, count(*) as view_count").
		Group("sofa_score_event_id").
		Order("view_count desc").
		Limit(limit).
		Scan(&stats)
	return stats, result.Error
}
