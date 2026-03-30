package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

type EventStats struct {
	SofaScoreEventId int64
	ViewCount        int64
}

func LogPlayback(deviceID uint, content string, startedAt int64) (*models.PlaybackLog, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var lastLog models.PlaybackLog
	db.Where("device_id=?", deviceID).Order("started_at DESC").First(&lastLog)
	if lastLog.ID != 0 {
		db.Model(&lastLog).Where("id = ?", lastLog.ID).Update("ended_at", startedAt)
	}

	log := &models.PlaybackLog{DeviceID: deviceID, Content: content, StartedAt: startedAt}
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
		Select("content, count(*) as view_count").
		Group("content").
		Order("view_count desc").
		Limit(limit).
		Scan(&stats)
	return stats, result.Error
}

func GetList(page, limit int) ([]*models.PlaybackLog, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	offset := (page - 1) * limit
	var stats []*models.PlaybackLog
	result := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&stats)
	return stats, result.Error
}

func TotalCount() int64 {
	db, err := database.GetDB()
	if err != nil {
		return 0
	}

	var count int64
	_ = db.Model(&models.PlaybackLog{}).Count(&count)
	return count
}
