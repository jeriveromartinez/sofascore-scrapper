package tournaments

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
)

type DeviceTournament struct {
	gorm.Model
	DeviceID     uint               `gorm:"not null;index:idx_device_tournament,unique" json:"device_id"`
	TournamentID uint               `gorm:"not null;index:idx_device_tournament,unique" json:"tournament_id"`
	Device       *devices.Device    `gorm:"foreignKey:DeviceID" json:"device,omitempty"`
	Tournament   *Tournament        `gorm:"foreignKey:TournamentID" json:"tournament,omitempty"`
}
