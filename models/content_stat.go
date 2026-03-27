package models

import "gorm.io/gorm"

const (
	PeriodTypeDay   = "day"
	PeriodTypeMonth = "month"
)

type ContentStat struct {
	*gorm.Model
	ContentHash string `gorm:"not null;index:idx_content_period,unique" json:"content_hash"`
	PeriodType  string `gorm:"not null;index:idx_content_period,unique" json:"period_type"`
	Seconds     int    `gorm:"not null" json:"seconds"`
	Views       int    `gorm:"not null" json:"views"`
}
