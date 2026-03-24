package models

import "gorm.io/gorm"

type PlaybackLog struct {
	gorm.Model
	DeviceID  uint   `gorm:"not null;index"`
	Content   string `gorm:"not null"`
	StartedAt int64
	EndedAt   int64
}
