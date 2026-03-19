package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

// GetAllTournaments retrieves all tournaments
func GetAllTournaments() ([]models.Tournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var tournaments []models.Tournament
	result := db.Order("slug ASC").Find(&tournaments)
	return tournaments, result.Error
}

// GetTournamentByID retrieves a tournament by ID
func GetTournamentByID(id uint) (*models.Tournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var tournament models.Tournament
	result := db.First(&tournament, id)
	return &tournament, result.Error
}

// CreateTournament creates a new tournament
func CreateTournament(name, slug string) (*models.Tournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	tournament := &models.Tournament{
		Name: name,
		Slug: slug,
	}
	result := db.Create(tournament)
	return tournament, result.Error
}

// UpdateTournament updates an existing tournament
func UpdateTournament(id uint, name, slug string) (*models.Tournament, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var tournament models.Tournament
	if err := db.First(&tournament, id).Error; err != nil {
		return nil, err
	}
	tournament.Name = name
	tournament.Slug = slug
	result := db.Save(&tournament)
	return &tournament, result.Error
}

// DeleteTournament deletes a tournament
func DeleteTournament(id uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Delete(&models.Tournament{}, id).Error
}
