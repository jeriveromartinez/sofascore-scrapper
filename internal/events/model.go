package events

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

type Team struct {
	gorm.Model
	TeamId         int64  `gorm:"column:team_id;uniqueIndex:idx_teams_team_id"`
	Name           string `gorm:"column:name;type:longtext"`
	LogoUrl        string `gorm:"column:logo_url;type:longtext"`
	PrimaryColor   string `gorm:"column:primary_color;type:longtext"`
	SecondaryColor string `gorm:"column:secondary_color;type:longtext"`
	TextColor      string `gorm:"column:text_color;type:longtext"`
}

type Event struct {
	gorm.Model
	SofaScoreEventId            int64  `gorm:"column:sofa_score_event_id;uniqueIndex:idx_events_sofa_score_event_id"`
	Sport                       string `gorm:"column:sport;type:longtext"`
	HomeScore                   int    `gorm:"column:home_score"`
	HomeTeamId                  int64  `gorm:"column:home_team_id;foreignKey:HomeTeamId;references:TeamId"`
	AwayScore                   int    `gorm:"column:away_score"`
	AwayTeamId                  int64  `gorm:"column:away_team_id;foreignKey:AwayTeamId;references:TeamId"`
	ScrapedAt                   int64  `gorm:"column:scraped_at"`
	StartTimestamp              int64  `gorm:"column:start_timestamp"`
	CurrentPeriodStartTimestamp int64  `gorm:"column:current_period_start_timestamp"`
	Slug                        string `gorm:"column:slug;type:longtext"`
	LeagueId                    uint   `gorm:"column:league_id;foreignKey:LeagueId;references:ID"`
	StatusType                  string `gorm:"column:status_type;size:32;not null;default:''"`
	HomeTeamModel               *Team                   `gorm:"foreignKey:HomeTeamId;references:TeamId" json:"teamHome,omitempty"`
	AwayTeamModel               *Team                   `gorm:"foreignKey:AwayTeamId;references:TeamId" json:"teamAway,omitempty"`
	League                      *tournaments.Tournament `gorm:"foreignKey:LeagueId;references:ID" json:"league,omitempty"`
}

func (Event) TableName() string {
	return "events"
}

type ScrapeBatch struct {
	Teams       []Team                   `json:"teams"`
	Tournaments []tournaments.Tournament `json:"tournaments"`
	Events      []Event                  `json:"events"`
}
