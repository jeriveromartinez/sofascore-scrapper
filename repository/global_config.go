package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	internalTournaments "github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
)

func globalConfigRepo() (*internalTournaments.GlobalConfigRepository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return internalTournaments.NewGlobalConfigRepository(db), nil
}

func GetGlobalTournamentConfig() ([]internalTournaments.GlobalTournamentConfig, error) {
	repo, err := globalConfigRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func RemoveGlobalTournamentConfig(tournamentID uint) error {
	repo, err := globalConfigRepo()
	if err != nil {
		return err
	}
	return repo.Remove(tournamentID)
}

func SetGlobalTournamentConfig(tournamentIDs []uint) ([]*internalTournaments.GlobalTournamentConfig, error) {
	repo, err := globalConfigRepo()
	if err != nil {
		return nil, err
	}
	return repo.Set(tournamentIDs)
}
