package events

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

type Team struct {
	gorm.Model
	TeamId         int64 `gorm:"uniqueIndex"`
	Name           string
	LogoUrl        string
	PrimaryColor   string
	SecondaryColor string
	TextColor      string
}

type Event struct {
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
	StatusType                  string
	HomeTeamModel               *Team                   `gorm:"foreignKey:HomeTeamId;references:TeamId" json:"teamHome,omitempty"`
	AwayTeamModel               *Team                   `gorm:"foreignKey:AwayTeamId;references:TeamId" json:"teamAway,omitempty"`
	League                      *tournaments.Tournament `gorm:"foreignKey:LeagueId" json:"league,omitempty"`
}

func (Event) TableName() string {
	return "events"
}

type ScrapeBatch struct {
	Teams       []Team                   `json:"teams"`
	Tournaments []tournaments.Tournament `json:"tournaments"`
	Events      []Event                  `json:"events"`
}
