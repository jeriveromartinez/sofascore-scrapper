package seeder

import (
	"context"
	"fmt"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SeedDefaultAdmin creates the default admin iff the users table is
// empty. Idempotent: returns nil on subsequent runs.
//
// Called automatically from app.New on every boot when SKIP_MIGRATE
// is not set. See docs/operations/runbook.md for changing the default
// password.
func SeedDefaultAdmin(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&users.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(DefaultAdminPassword), defaultAdminBcryptCost)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}
	return db.WithContext(ctx).Create(&users.User{
		Email:    DefaultAdminEmail,
		Password: string(hashed),
		Role:     users.RoleAdmin,
	}).Error
}
