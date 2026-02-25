package models

import (
	"time"

	"gorm.io/gorm"
)

// SportEvent represents a sports event scraped from Sofascore.
type SportEvent struct {
	gorm.Model
	Sport         string    `gorm:"type:varchar(100)" json:"sport"`
	Tournament    string    `gorm:"type:varchar(255)" json:"tournament"`
	HomeTeam      string    `gorm:"type:varchar(255)" json:"home_team"`
	AwayTeam      string    `gorm:"type:varchar(255)" json:"away_team"`
	HomeScore     string    `gorm:"type:varchar(20)" json:"home_score"`
	AwayScore     string    `gorm:"type:varchar(20)" json:"away_score"`
	Status        string    `gorm:"type:varchar(100)" json:"status"`
	StartTime     string    `gorm:"type:varchar(50)" json:"start_time"`
	ScrapedAt     time.Time `gorm:"autoCreateTime" json:"scraped_at"`
	RawText       string    `gorm:"type:text" json:"raw_text"`
}
