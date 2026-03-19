package models

import "gorm.io/gorm"

type SofaScoreEvent struct {
	gorm.Model
	SofaScoreEventId            int64 `gorm:"uniqueIndex"`
	Sport                       string
	HomeScore                   int
	HomeTeamId                  int64
	AwayScore                   int
	AwayTeamId                  int64
	ScrapedAt                   int64
	StartTimestamp              int64
	CurrentPeriodStartTimestamp int64
	Slug                        string
	LeagueId                    uint
	HomeTeamModel               *Team       `gorm:"foreignKey:HomeTeamId;references:TeamId" json:"teamHome,omitempty"`
	AwayTeamModel               *Team       `gorm:"foreignKey:AwayTeamId;references:TeamId" json:"teamAway,omitempty"`
	League                      *Tournament `gorm:"foreignKey:LeagueId" json:"league,omitempty"`
}

func (SofaScoreEvent) TableName() string {
	return "events"
}
