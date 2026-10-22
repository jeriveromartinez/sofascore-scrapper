package tournaments

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll(ctx context.Context) ([]Tournament, error) {
	var tournaments []Tournament
	result := r.db.WithContext(ctx).Order("slug ASC").Find(&tournaments)
	return tournaments, result.Error
}

func (r *Repository) ListPage(ctx context.Context, slug string, id uint, limit int) ([]Tournament, bool, error) {
	query := r.db.WithContext(ctx).Order("slug ASC, id ASC")
	if slug != "" {
		query = query.Where("slug > ? OR (slug = ? AND id > ?)", slug, slug, id)
	}
	var rows []Tournament
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

func (r *Repository) GetByID(ctx context.Context, id uint) (*Tournament, error) {
	var tournament Tournament
	result := r.db.WithContext(ctx).First(&tournament, id)
	return &tournament, result.Error
}

// maxIDAllocationAttempts bounds the retry loop in Create. Two admins
// creating a tournament at the same instant can pick the same
// candidate id; the loser retries with the next one.
const maxIDAllocationAttempts = 5

// Create inserts an admin-created tournament.
//
// The primary key is allocated from the band reserved for admin rows
// (see model.go) instead of the table's auto-increment counter, so an
// admin tournament can never collide with an upstream league row.
func (r *Repository) Create(ctx context.Context, name, slug string) (*Tournament, error) {
	for range maxIDAllocationAttempts {
		id, err := r.nextAdminTournamentID(ctx)
		if err != nil {
			return nil, err
		}
		tournament := &Tournament{Model: gorm.Model{ID: id}, Name: name, Slug: slug}
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(tournament)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected > 0 {
			return tournament, nil
		}
	}
	return nil, fmt.Errorf("could not allocate a tournament id in the reserved admin range")
}

// nextAdminTournamentID returns the next free primary key inside the
// reserved admin band.
//
// The lookup is unscoped: a soft-deleted tournament still occupies its
// primary key, so counting only the live rows would hand the same id
// out again and the insert would collide with the deleted row.
func (r *Repository) nextAdminTournamentID(ctx context.Context) (uint, error) {
	var maxID sql.NullInt64
	row := r.db.WithContext(ctx).Unscoped().Model(&Tournament{}).
		Where("id >= ? AND id < ?", AdminTournamentIDBase, AdminTournamentIDCeiling).
		Select("MAX(id)").
		Row()
	if err := row.Scan(&maxID); err != nil {
		return 0, err
	}
	if !maxID.Valid {
		return AdminTournamentIDBase, nil
	}
	next := uint(maxID.Int64) + 1
	if next >= AdminTournamentIDCeiling {
		return 0, fmt.Errorf("reserved tournament id range is exhausted")
	}
	return next, nil
}

func (r *Repository) Update(ctx context.Context, id uint, name, slug string) (*Tournament, error) {
	var tournament Tournament
	if err := r.db.WithContext(ctx).First(&tournament, id).Error; err != nil {
		return nil, err
	}
	tournament.Name = name
	tournament.Slug = slug
	result := r.db.WithContext(ctx).Save(&tournament)
	return &tournament, result.Error
}

func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Tournament{}, id).Error
}
