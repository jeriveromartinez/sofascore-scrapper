package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	internalTournaments "github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
)

func deviceAssignmentsRepo() (*internalTournaments.DeviceAssignmentsRepository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return internalTournaments.NewDeviceAssignmentsRepository(db), nil
}

func GetDeviceTournaments(deviceID uint) ([]internalTournaments.DeviceTournament, error) {
	repo, err := deviceAssignmentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByDevice(deviceID)
}

func GetAllDeviceTournaments() ([]internalTournaments.DeviceTournament, error) {
	repo, err := deviceAssignmentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func AssignTournamentToDevice(deviceID, tournamentID uint) (*internalTournaments.DeviceTournament, error) {
	repo, err := deviceAssignmentsRepo()
	if err != nil {
		return nil, err
	}
	return repo.Assign(deviceID, tournamentID)
}

func RemoveTournamentFromDevice(deviceID, tournamentID uint) error {
	repo, err := deviceAssignmentsRepo()
	if err != nil {
		return err
	}
	return repo.Remove(deviceID, tournamentID)
}

func SetDeviceTournaments(deviceID uint, tournamentIDs []uint) error {
	repo, err := deviceAssignmentsRepo()
	if err != nil {
		return err
	}
	return repo.Set(deviceID, tournamentIDs)
}
