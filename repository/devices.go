package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	internalDevices "github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
)

func devicesRepo() (*internalDevices.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return internalDevices.NewRepository(db), nil
}

func RegisterDevice(userID *uint, token, platform, name, version string) (*internalDevices.Device, error) {
	repo, err := devicesRepo()
	if err != nil {
		return nil, err
	}
	return repo.Register(userID, token, platform, name, version)
}

func UpdateDeviceLastSeen(token string) error {
	repo, err := devicesRepo()
	if err != nil {
		return err
	}
	return repo.UpdateLastSeen(token)
}

func GetDevices(page, limit uint) ([]internalDevices.Device, int64, error) {
	repo, err := devicesRepo()
	if err != nil {
		return nil, 0, err
	}
	return repo.GetDevices(page, limit)
}

func GetAllDevices() ([]internalDevices.Device, error) {
	repo, err := devicesRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func UpdateDevice(token, platform, name string) (*internalDevices.Device, error) {
	repo, err := devicesRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(token, platform, name)
}
