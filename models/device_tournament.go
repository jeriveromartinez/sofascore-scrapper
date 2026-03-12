package models

import "gorm.io/gorm"

// DeviceTournament represents the many-to-many relationship between Device and Tournament
type DeviceTournament struct {
	gorm.Model
	DeviceID     uint       `gorm:"not null;index:idx_device_tournament,unique" json:"device_id"`
	TournamentID uint       `gorm:"not null;index:idx_device_tournament,unique" json:"tournament_id"`
	Device       *Device    `gorm:"foreignKey:DeviceID" json:"device,omitempty"`
	Tournament   *Tournament `gorm:"foreignKey:TournamentID" json:"tournament,omitempty"`
}
