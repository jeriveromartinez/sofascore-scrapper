package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

// GetGlobalTournamentConfig retrieves all global tournament configurations
func GetGlobalTournamentConfig() ([]models.GlobalTournamentConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var configs []models.GlobalTournamentConfig
	result := db.Preload("Tournament").Find(&configs)
	return configs, result.Error
}

// AddGlobalTournamentConfig adds a tournament to global configuration
func AddGlobalTournamentConfig(tournamentID uint) (*models.GlobalTournamentConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	config := &models.GlobalTournamentConfig{
		TournamentID: tournamentID,
	}
	result := db.Create(config)
	return config, result.Error
}

// RemoveGlobalTournamentConfig removes a tournament from global configuration
func RemoveGlobalTournamentConfig(tournamentID uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Where("tournament_id = ?", tournamentID).Delete(&models.GlobalTournamentConfig{}).Error
}

// SetGlobalTournamentConfig sets the global tournament configuration (replaces all existing)
func SetGlobalTournamentConfig(tournamentIDs []uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	// Start transaction
	tx := db.Begin()

	// Delete all existing configurations
	if err := tx.Where("1 = 1").Delete(&models.GlobalTournamentConfig{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Create new configurations
	for _, tournamentID := range tournamentIDs {
		config := &models.GlobalTournamentConfig{
			TournamentID: tournamentID,
		}
		if err := tx.Create(config).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
