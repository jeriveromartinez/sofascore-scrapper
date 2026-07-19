package playback

import (
	"context"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Start(ctx context.Context, deviceID uint, content string, startedAt int64) (*PlaybackLog, error) {
	var created PlaybackLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device devices.Device
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
			return err
		}
		if err := tx.Model(&PlaybackLog{}).Where("device_id = ? AND ended_at = 0", deviceID).Update("ended_at", startedAt).Error; err != nil {
			return err
		}
		created = PlaybackLog{DeviceID: deviceID, Content: content, StartedAt: startedAt}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		return tx.Model(&devices.Device{}).Where("id = ?", deviceID).Update("last_seen", startedAt).Error
	})
	return &created, err
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

func (r *Repository) ListPage(ctx context.Context, createdAtStr string, id uint, limit int) ([]PlaybackLog, bool, error) {
	query := r.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if createdAtStr != "" {
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, false, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", createdAt, createdAt, id)
	}
	var rows []PlaybackLog
	err := query.Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}
