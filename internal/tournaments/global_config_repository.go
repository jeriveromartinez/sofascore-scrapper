package tournaments

import "gorm.io/gorm"

type GlobalConfigRepository struct {
	db *gorm.DB
}

func NewGlobalConfigRepository(db *gorm.DB) *GlobalConfigRepository {
	return &GlobalConfigRepository{db: db}
}

func (r *GlobalConfigRepository) GetAll() ([]GlobalTournamentConfig, error) {
	configs := make([]GlobalTournamentConfig, 0)
	result := r.db.Preload("Tournament").Find(&configs)
	return configs, result.Error
}

func (r *GlobalConfigRepository) Remove(tournamentID uint) error {
	return r.db.Where("tournament_id = ?", tournamentID).Unscoped().Delete(&GlobalTournamentConfig{}).Error
}

func (r *GlobalConfigRepository) Set(tournamentIDs []uint) ([]*GlobalTournamentConfig, error) {
	tx := r.db.Begin()
	if err := tx.Where("1 = 1").Unscoped().Delete(&GlobalTournamentConfig{}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	for _, tournamentID := range tournamentIDs {
		config := &GlobalTournamentConfig{TournamentID: tournamentID}
		if err := tx.Create(config).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	var configs []*GlobalTournamentConfig
	if err := r.db.Preload("Tournament").Find(&configs).Error; err != nil {
		return nil, err
	}

	return configs, nil
}
