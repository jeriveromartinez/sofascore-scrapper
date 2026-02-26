package models

import "gorm.io/gorm"

type SofaScoreEvent struct {
	gorm.Model
	HomeTeam                    string
	HomeScore                   int
	AwayTeam                    string
	AwayScore                   int
	ScrapedAt                   int64
	Category                    string
	StartTimestamp              int64
	CurrentPeriodStartTimestamp int64
	Slug                        string
	LeagueName                  string
}
