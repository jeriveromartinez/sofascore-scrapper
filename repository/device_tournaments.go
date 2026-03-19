package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

// GetDeviceTournaments retrieves all tournament assignments for a device
func GetDeviceTournaments(deviceID uint) ([]models.DeviceTournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var deviceTournaments []models.DeviceTournament
	result := db.Where("device_id = ?", deviceID).Preload("Tournament").Find(&deviceTournaments)
	return deviceTournaments, result.Error
}

// GetAllDeviceTournaments retrieves all device-tournament associations
func GetAllDeviceTournaments() ([]models.DeviceTournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var deviceTournaments []models.DeviceTournament
	result := db.Preload("Device").Preload("Tournament").Find(&deviceTournaments)
	return deviceTournaments, result.Error
}

// AssignTournamentToDevice creates a device-tournament association
func AssignTournamentToDevice(deviceID, tournamentID uint) (*models.DeviceTournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	deviceTournament := &models.DeviceTournament{
		DeviceID:     deviceID,
		TournamentID: tournamentID,
	}
	result := db.Create(deviceTournament)
	return deviceTournament, result.Error
}

// RemoveTournamentFromDevice removes a device-tournament association
func RemoveTournamentFromDevice(deviceID, tournamentID uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Where("device_id = ? AND tournament_id = ?", deviceID, tournamentID).Delete(&models.DeviceTournament{}).Error
}

// SetDeviceTournaments sets the tournaments for a device (replaces all existing)
func SetDeviceTournaments(deviceID uint, tournamentIDs []uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	// Start transaction
	tx := db.Begin()

	// Delete existing associations
	if err := tx.Where("device_id = ?", deviceID).Unscoped().Delete(&models.DeviceTournament{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Create new associations
	for _, tournamentID := range tournamentIDs {
		deviceTournament := &models.DeviceTournament{DeviceID: deviceID, TournamentID: tournamentID}
		if err := tx.Create(deviceTournament).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
