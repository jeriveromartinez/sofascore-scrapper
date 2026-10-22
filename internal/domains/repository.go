package domains

import (
	"context"

	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll(ctx context.Context) ([]Domain, error) {
	domains := make([]Domain, 0)
	result := r.db.WithContext(ctx).Preload("User").Order("domain ASC").Find(&domains)
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

func (r *Repository) GetByID(ctx context.Context, id uint) (*Domain, error) {
	var domain Domain
	result := r.db.WithContext(ctx).Preload("User").First(&domain, id)
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

func (r *Repository) Create(ctx context.Context, domain string, userID uint) (*Domain, error) {
	if err := r.db.WithContext(ctx).First(&users.User{}, userID).Error; err != nil {
		return nil, err
	}

	record := &Domain{Domain: domain, UserID: userID}
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Preload("User").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	return record, nil
}

// Update changes the domain string for an existing record. Ownership
// (UserID) is deliberately NOT writable through this path: a generic
// update endpoint that accepts a fresh UserID lets a future caller
// (or a future regression in the auth middleware) reassign the
// record to any user. If ownership reassignment becomes a product
// feature, expose it as a dedicated TransferOwnership(ctx, caller,
// domainID, newUserID) method that checks the caller is admin and
// logs the transfer — never as a side effect of an "edit" path.
func (r *Repository) Update(ctx context.Context, id uint, domain string) (*Domain, error) {
	var record Domain
	if err := r.db.WithContext(ctx).First(&record, id).Error; err != nil {
		return nil, err
	}

	record.Domain = domain
	if err := r.db.WithContext(ctx).Save(&record).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Preload("User").First(&record, record.ID).Error; err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *Repository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).First(&Domain{}, id).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&Domain{}, id).Error
}
