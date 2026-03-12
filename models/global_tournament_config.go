package models

import "gorm.io/gorm"

// GlobalTournamentConfig represents the global configuration for tournaments
// that should be shown to devices without specific tournament assignments
type GlobalTournamentConfig struct {
	gorm.Model
	TournamentID uint       `gorm:"not null;uniqueIndex" json:"tournament_id"`
	Tournament   *Tournament `gorm:"foreignKey:TournamentID" json:"tournament,omitempty"`
}
