package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	PeriodTypeDay   = "day"
	PeriodTypeMonth = "month"
)

type ContentStat struct {
	gorm.Model
	ContentHash string    `gorm:"not null;index:idx_content_period,unique" json:"content_hash"`
	PeriodType  string    `gorm:"not null;index:idx_content_period,unique" json:"period_type"`
	PeriodStart time.Time `gorm:"not null;index:idx_content_period,unique" json:"period_start"`
	Seconds     int       `gorm:"not null" json:"seconds"`
	Views       int       `gorm:"not null" json:"views"`
}
