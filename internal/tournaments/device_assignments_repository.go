package tournaments

import "gorm.io/gorm"

type DeviceAssignmentsRepository struct {
	db *gorm.DB
}

func NewDeviceAssignmentsRepository(db *gorm.DB) *DeviceAssignmentsRepository {
	return &DeviceAssignmentsRepository{db: db}
}

func (r *DeviceAssignmentsRepository) GetByDevice(deviceID uint) ([]DeviceTournament, error) {
	var deviceTournaments []DeviceTournament
	result := r.db.Where("device_id = ?", deviceID).Preload("Tournament").Find(&deviceTournaments)
	return deviceTournaments, result.Error
}

func (r *DeviceAssignmentsRepository) GetAll() ([]DeviceTournament, error) {
	var deviceTournaments []DeviceTournament
	result := r.db.Preload("Device").Preload("Tournament").Find(&deviceTournaments)
	return deviceTournaments, result.Error
}

func (r *DeviceAssignmentsRepository) Assign(deviceID, tournamentID uint) (*DeviceTournament, error) {
	deviceTournament := &DeviceTournament{DeviceID: deviceID, TournamentID: tournamentID}
	result := r.db.Create(deviceTournament)
	return deviceTournament, result.Error
}

func (r *DeviceAssignmentsRepository) Remove(deviceID, tournamentID uint) error {
	return r.db.Where("device_id = ? AND tournament_id = ?", deviceID, tournamentID).Delete(&DeviceTournament{}).Error
}

func (r *DeviceAssignmentsRepository) Set(deviceID uint, tournamentIDs []uint) error {
	tx := r.db.Begin()

	if err := tx.Where("device_id = ?", deviceID).Unscoped().Delete(&DeviceTournament{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, tournamentID := range tournamentIDs {
		dt := &DeviceTournament{DeviceID: deviceID, TournamentID: tournamentID}
		if err := tx.Create(dt).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
