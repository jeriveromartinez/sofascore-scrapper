package models

import "gorm.io/gorm"

type PlaybackLog struct {
	gorm.Model
	DeviceID         uint  `gorm:"not null;index"`
	SofaScoreEventId int64 `gorm:"not null;index"`
	StartedAt        int64
	EndedAt          int64
}
