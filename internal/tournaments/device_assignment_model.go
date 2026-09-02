package tournaments

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
)

type DeviceTournament struct {
	gorm.Model
	DeviceID     uint            `gorm:"column:device_id;not null;uniqueIndex:idx_device_tournament,priority:1;foreignKey:DeviceID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE" json:"device_id"`
	TournamentID uint            `gorm:"column:tournament_id;not null;uniqueIndex:idx_device_tournament,priority:2;foreignKey:TournamentID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE" json:"tournament_id"`
	Device       *devices.Device `gorm:"foreignKey:DeviceID;references:ID" json:"device,omitempty"`
	Tournament   *Tournament     `gorm:"foreignKey:TournamentID;references:ID" json:"tournament,omitempty"`
}
