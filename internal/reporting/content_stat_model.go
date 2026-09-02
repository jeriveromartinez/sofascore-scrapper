package reporting

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
	ContentHash string    `gorm:"column:content_hash;size:191;not null;index:idx_content_period,priority:1,unique" json:"content_hash"`
	PeriodType  string    `gorm:"column:period_type;size:191;not null;index:idx_content_period,priority:2,unique" json:"period_type"`
	PeriodStart time.Time `gorm:"column:period_start;not null;index:idx_content_period,priority:3,unique" json:"period_start"`
	Seconds     int       `gorm:"not null" json:"seconds"`
	Views       int       `gorm:"not null" json:"views"`
}
