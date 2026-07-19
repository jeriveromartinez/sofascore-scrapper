package tournaments

import (
	"context"

	"gorm.io/gorm"
)

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

func (r *DeviceAssignmentsRepository) ListPage(ctx context.Context, deviceID, tournamentID, id uint, limit int) ([]DeviceTournament, bool, error) {
	query := r.db.WithContext(ctx).Preload("Device").Preload("Tournament").
		Order("device_id ASC, tournament_id ASC, id ASC")

	if deviceID > 0 || tournamentID > 0 || id > 0 {
		query = query.Where(
			"device_id > ? OR (device_id = ? AND tournament_id > ?) OR (device_id = ? AND tournament_id = ? AND id > ?)",
			deviceID, deviceID, tournamentID, deviceID, tournamentID, id,
		)
	}

	var rows []DeviceTournament
	err := query.Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
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
