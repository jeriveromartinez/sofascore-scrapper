package models

import (
	"fmt"

	"gorm.io/gorm"
)

type SofaScoreEvent struct {
	gorm.Model
	SofaScoreEventId            int64  `gorm:"uniqueIndex"`
	Sport                       string
	HomeTeam                    string
	HomeScore                   int
	HomeTeamId                  int64
	AwayTeam                    string
	AwayScore                   int
	AwayTeamId                  int64
	ScrapedAt                   int64
	Category                    string
	StartTimestamp              int64
	CurrentPeriodStartTimestamp int64
	Slug                        string
	LeagueName                  string
}

func (s SofaScoreEvent) GetHomeTeamLogo() string {
	return "https://img.sofascore.com/api/v1/team/" + fmt.Sprint(s.HomeTeamId) + "/image"
}

func (s SofaScoreEvent) GetAwayTeamLogo() string {
	return "https://img.sofascore.com/api/v1/team/" + fmt.Sprint(s.AwayTeamId) + "/image"
}
