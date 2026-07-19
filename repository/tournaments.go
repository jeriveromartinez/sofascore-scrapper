package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	internalTournaments "github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
)

func tournamentsRepo() (*internalTournaments.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return internalTournaments.NewRepository(db), nil
}

func GetAllTournaments() ([]internalTournaments.Tournament, error) {
	repo, err := tournamentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func GetTournamentByID(id uint) (*internalTournaments.Tournament, error) {
	repo, err := tournamentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByID(id)
}

func CreateTournament(name, slug string) (*internalTournaments.Tournament, error) {
	repo, err := tournamentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(name, slug)
}

func UpdateTournament(id uint, name, slug string) (*internalTournaments.Tournament, error) {
	repo, err := tournamentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(id, name, slug)
}

func DeleteTournament(id uint) error {
	repo, err := tournamentsRepo()
	if err != nil {
		return err
	}
	return repo.Delete(id)
}
