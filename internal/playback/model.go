package playback

import "gorm.io/gorm"

type PlaybackLog struct {
	gorm.Model
	DeviceID  uint   `gorm:"column:device_id;not null;index:idx_playback_logs_device_id"`
	Content   string `gorm:"column:content;type:longtext;not null"`
	StartedAt int64
	EndedAt   int64
}
