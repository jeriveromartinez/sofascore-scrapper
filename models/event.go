package models

import (
	"time"

	"gorm.io/gorm"
)

// SportEvent represents a sports event scraped from Sofascore.
type SportEvent struct {
	gorm.Model
	DataID         string    `gorm:"type:varchar(100);index" json:"data_id"`
	Sport          string    `gorm:"type:varchar(100)" json:"sport"`
	Tournament     string    `gorm:"type:varchar(255)" json:"tournament"`
	HomeTeam       string    `gorm:"type:varchar(255)" json:"home_team"`
	HomeTeamImage  string    `gorm:"type:varchar(500)" json:"home_team_image"`
	AwayTeam       string    `gorm:"type:varchar(255)" json:"away_team"`
	AwayTeamImage  string    `gorm:"type:varchar(500)" json:"away_team_image"`
	HomeScore      string    `gorm:"type:varchar(20)" json:"home_score"`
	AwayScore      string    `gorm:"type:varchar(20)" json:"away_score"`
	Status         string    `gorm:"type:varchar(100)" json:"status"`
	StartTime      string    `gorm:"type:varchar(50)" json:"start_time"`
	ScrapedAt      time.Time `gorm:"autoCreateTime" json:"scraped_at"`
	RawText        string    `gorm:"type:text" json:"raw_text"`
}
