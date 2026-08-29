package domains

import (
	"context"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll() ([]Domain, error) {
	domains := make([]Domain, 0)
	result := r.db.Preload("User").Order("domain ASC").Find(&domains)
	return domains, result.Error
}

func (r *Repository) ListPage(ctx context.Context, domainStr string, id uint, limit int) ([]Domain, bool, error) {
	query := r.db.WithContext(ctx).Preload("User").Order("domain ASC, id ASC")
	if domainStr != "" {
		query = query.Where("domain > ? OR (domain = ? AND id > ?)", domainStr, domainStr, id)
	}
	var rows []Domain
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

func (r *Repository) GetByID(id uint) (*Domain, error) {
	var domain Domain
	result := r.db.Preload("User").First(&domain, id)
	return &domain, result.Error
}

// ListByUser returns every domain owned by the user. Used by the
// push service to validate that a requested domain_id belongs to
// the caller before dispatching the push.
func (r *Repository) ListByUser(ctx context.Context, userID uint) ([]Domain, error) {
	var rows []Domain
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error
	return rows, err
}

func (r *Repository) Create(domain string, userID uint) (*Domain, error) {
	if err := r.db.First(&users.User{}, userID).Error; err != nil {
		return nil, err
	}

	record := &Domain{Domain: domain, UserID: userID}
	if err := r.db.Create(record).Error; err != nil {
		return nil, err
	}

	if err := r.db.Preload("User").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	return record, nil
}

func (r *Repository) Update(id uint, domain string, userID uint) (*Domain, error) {
	var record Domain
	if err := r.db.First(&record, id).Error; err != nil {
		return nil, err
	}

	if err := r.db.First(&users.User{}, userID).Error; err != nil {
		return nil, err
	}

	record.Domain = domain
	record.UserID = userID
	if err := r.db.Save(&record).Error; err != nil {
		return nil, err
	}

	if err := r.db.Preload("User").First(&record, record.ID).Error; err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *Repository) Delete(id uint) error {
	if err := r.db.First(&Domain{}, id).Error; err != nil {
		return err
	}
	return r.db.Delete(&Domain{}, id).Error
}
