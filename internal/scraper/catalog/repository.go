// internal/scraper/catalog/repository.go
package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("catalog: not found")
var ErrDuplicate = errors.New("catalog: duplicate (source, source_league_id)")

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, sl *ScraperLeague) error {
	if err := r.db.WithContext(ctx).Create(sl).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s/%s", ErrDuplicate, sl.Source, sl.SourceLeagueId)
		}
		return err
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, id uint, fields map[string]any) error {
	res := r.db.WithContext(ctx).Model(&ScraperLeague{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uint) (*ScraperLeague, error) {
	var sl ScraperLeague
	if err := r.db.WithContext(ctx).First(&sl, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sl, nil
}

type ListFilters struct {
	Enabled *bool
	Source  string
	Country string
	Query   string
	Page    int
	Limit   int
}

func (r *Repository) List(ctx context.Context, f ListFilters) ([]ScraperLeague, int64, error) {
	q := r.db.WithContext(ctx).Model(&ScraperLeague{})
	if f.Enabled != nil {
		q = q.Where("enabled = ?", *f.Enabled)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.Country != "" {
		q = q.Where("country = ?", f.Country)
	}
	if f.Query != "" {
		q = q.Where("name LIKE ?", "%"+f.Query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page < 1 {
		f.Page = 1
	}
	var out []ScraperLeague
	if err := q.Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Order("name ASC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) SoftDelete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&ScraperLeague{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ActiveLeagues(ctx context.Context) ([]scraper.LeagueRef, error) {
	var rows []ScraperLeague
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]scraper.LeagueRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, scraper.LeagueRef{
			Source:         r.Source,
			SourceLeagueId: r.SourceLeagueId,
			Name:           r.Name,
			Country:        r.Country,
			Sport:          r.Sport,
		})
	}
	return out, nil
}

func isUniqueViolation(err error) bool {
	// MariaDB: error 1062
	return err != nil && (containsAll(err.Error(), "Error 1062") || containsAll(err.Error(), "UNIQUE constraint failed"))
}

func containsAll(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
