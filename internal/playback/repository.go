package playback

import (
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Log(deviceID uint, content string, startedAt int64) (*PlaybackLog, error) {
	var lastLog PlaybackLog
	r.db.Where("device_id=?", deviceID).Order("started_at DESC").First(&lastLog)
	if lastLog.ID != 0 {
		r.db.Model(&lastLog).Where("id = ?", lastLog.ID).Update("ended_at", startedAt)
	}

	log := &PlaybackLog{DeviceID: deviceID, Content: content, StartedAt: startedAt}
	result := r.db.Create(log)
	return log, result.Error
}

func (r *Repository) UpdateEnd(id uint, endedAt int64) error {
	return r.db.Model(&PlaybackLog{}).Where("id = ?", id).Update("ended_at", endedAt).Error
}

func (r *Repository) GetList(page, limit int) ([]*PlaybackLog, error) {
	offset := (page - 1) * limit
	var logs []*PlaybackLog
	result := r.db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs)
	return logs, result.Error
}

func (r *Repository) TotalCount() int64 {
	var count int64
	_ = r.db.Model(&PlaybackLog{}).Count(&count)
	return count
}
