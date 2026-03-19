package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func GetGlobalTournamentConfig() ([]models.GlobalTournamentConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	configs := make([]models.GlobalTournamentConfig, 0)
	result := db.Preload("Tournament").Find(&configs)
	return configs, result.Error
}

func RemoveGlobalTournamentConfig(tournamentID uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Where("tournament_id = ?", tournamentID).Unscoped().Delete(&models.GlobalTournamentConfig{}).Error
}

func SetGlobalTournamentConfig(tournamentIDs []uint) ([]*models.GlobalTournamentConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	tx := db.Begin()
	if err := tx.Where("1 = 1").Unscoped().Delete(&models.GlobalTournamentConfig{}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	for _, tournamentID := range tournamentIDs {
		config := &models.GlobalTournamentConfig{
			TournamentID: tournamentID,
		}
		if err := tx.Create(config).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	var configs []*models.GlobalTournamentConfig
	if err := db.Preload("Tournament").Find(&configs).Error; err != nil {
		return nil, err
	}

	return configs, nil
}
