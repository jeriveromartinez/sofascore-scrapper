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
	return r.db.Unscoped().Where("device_id = ? AND tournament_id = ?", deviceID, tournamentID).Delete(&DeviceTournament{}).Error
}

func (r *DeviceAssignmentsRepository) Set(deviceID uint, tournamentIDs []uint) error {
	// Replaces the entire per-device tournament set in a single
	// transaction. The previous implementation issued one INSERT per
	// tournament (N+1 round-trips): on the admin "assign all" path a
	// device with 100 tournaments cost 100 round-trips and 100 WAL
	// flushes. Bulk-inserting via a single CREATE keeps the whole
	// operation atomic and bounded to two SQL round-trips (DELETE +
	// one multi-row INSERT, or just the DELETE when the new set is
	// empty).
	//
	// db.Transaction wraps the body so a panic mid-loop still triggers
	// Rollback instead of leaking an open transaction on the pool.
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", deviceID).Unscoped().Delete(&DeviceTournament{}).Error; err != nil {
			return err
		}
		if len(tournamentIDs) == 0 {
			return nil
		}
		rows := make([]DeviceTournament, 0, len(tournamentIDs))
		for _, tid := range tournamentIDs {
			rows = append(rows, DeviceTournament{DeviceID: deviceID, TournamentID: tid})
		}
		return tx.Create(&rows).Error
	})
}
