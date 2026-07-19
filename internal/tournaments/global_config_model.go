package tournaments

import "gorm.io/gorm"

type GlobalTournamentConfig struct {
	gorm.Model
	TournamentID uint        `gorm:"not null;uniqueIndex" json:"tournament_id"`
	Tournament   *Tournament `gorm:"foreignKey:TournamentID" json:"tournament,omitempty"`
}
