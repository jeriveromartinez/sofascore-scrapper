package tournaments

import (
	"context"

	"gorm.io/gorm"
)

type GlobalConfigRepository struct {
	db *gorm.DB
}

func NewGlobalConfigRepository(db *gorm.DB) *GlobalConfigRepository {
	return &GlobalConfigRepository{db: db}
}

func (r *GlobalConfigRepository) GetAll(ctx context.Context) ([]GlobalTournamentConfig, error) {
	configs := make([]GlobalTournamentConfig, 0)
	result := r.db.WithContext(ctx).Preload("Tournament").Find(&configs)
	return configs, result.Error
}

func (r *GlobalConfigRepository) Remove(ctx context.Context, tournamentID uint) error {
	return r.db.WithContext(ctx).Where("tournament_id = ?", tournamentID).Unscoped().Delete(&GlobalTournamentConfig{}).Error
}

// Set replaces the entire global tournament configuration with the
// provided list, all inside a single transaction so the operator
// never observes an empty intermediate state. The previous
// implementation used manual Begin/Commit and a per-row insert
// loop; the new one uses Transaction(...) (which rolls back
// automatically on panic and ignores the rollback error on the
// non-error paths) and a single batched INSERT for the new rows.
func (r *GlobalConfigRepository) Set(ctx context.Context, tournamentIDs []uint) ([]*GlobalTournamentConfig, error) {
	rows := make([]*GlobalTournamentConfig, 0, len(tournamentIDs))
	for _, id := range tournamentIDs {
		rows = append(rows, &GlobalTournamentConfig{TournamentID: id})
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Unscoped().Delete(&GlobalTournamentConfig{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(rows).Error
	})
	if err != nil {
		return nil, err
	}

	var configs []*GlobalTournamentConfig
	if err := r.db.WithContext(ctx).Preload("Tournament").Find(&configs).Error; err != nil {
		return nil, err
	}

	return configs, nil
}
