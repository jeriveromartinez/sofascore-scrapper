package tournaments

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll() ([]Tournament, error) {
	var tournaments []Tournament
	result := r.db.Order("slug ASC").Find(&tournaments)
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

func (r *Repository) GetByID(id uint) (*Tournament, error) {
	var tournament Tournament
	result := r.db.First(&tournament, id)
	return &tournament, result.Error
}

func (r *Repository) Create(name, slug string) (*Tournament, error) {
	tournament := &Tournament{Name: name, Slug: slug}
	result := r.db.Create(tournament)
	return tournament, result.Error
}

func (r *Repository) Update(id uint, name, slug string) (*Tournament, error) {
	var tournament Tournament
	if err := r.db.First(&tournament, id).Error; err != nil {
		return nil, err
	}
	tournament.Name = name
	tournament.Slug = slug
	result := r.db.Save(&tournament)
	return &tournament, result.Error
}

func (r *Repository) Delete(id uint) error {
	return r.db.Delete(&Tournament{}, id).Error
}
