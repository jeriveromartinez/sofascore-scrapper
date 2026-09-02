package tournaments

import "gorm.io/gorm"

type GlobalTournamentConfig struct {
	gorm.Model
	TournamentID uint        `gorm:"column:tournament_id;not null;uniqueIndex:idx_global_tournament_configs_tournament_id;foreignKey:TournamentID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE" json:"tournament_id"`
	Tournament   *Tournament `gorm:"foreignKey:TournamentID;references:ID" json:"tournament,omitempty"`
}
