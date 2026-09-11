package seeder

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	// DefaultAdminEmail is the email seeded on first boot.
	DefaultAdminEmail = "admin@local"

	// DefaultAdminPassword is the plaintext password seeded on first
	// boot. It is bcrypt-hashed before being persisted. Operators
	// MUST change it on first login.
	DefaultAdminPassword = "admin1234"

	// defaultAdminBcryptCost matches auth.BcryptCost so the seeded
	// admin password lands at the current cost without a lazy rehash
	// on first login.
	defaultAdminBcryptCost = auth.BcryptCost
)

// SeedDefaultAdmin runs the first-boot seed. On the first boot after
// the FotMob model reset, it detects legacy SofaScore events (rows
// with source = "sofascore" or with the empty default source) and
// wipes events/teams/tournaments before creating the default admin
// user. The guard is idempotent: once the legacy rows are gone,
// subsequent boots are no-ops for the truncation branch.
//
// The legacy-truncation step is skipped if the events table does not
// exist yet (e.g. when a test calls SeedDefaultAdmin against a fresh
// DB without AutoMigrateAll), so the function is safe to call from
// both production boot and unit-test paths. Logging goes through
// slog.Default(); callers cannot inject a custom logger.
//
// Called automatically from app.New on every boot when SKIP_MIGRATE
// is not set, and from cmd/server/main.go for the migrate-only
// command-line path. See docs/operations/runbook.md for changing the
// default password.
func SeedDefaultAdmin(ctx context.Context, db *gorm.DB) error {
	// Legacy-data guard. HasTable is a no-op fast path when the
	// events table hasn't been migrated yet (e.g. unit tests that
	// only migrate users).
	if db.Migrator().HasTable(&events.Event{}) {
		var legacyCount int64
		if err := db.WithContext(ctx).Model(&events.Event{}).
			Where("source = ? OR source = ''", "sofascore").
			Count(&legacyCount).Error; err != nil {
			return fmt.Errorf("count legacy events: %w", err)
		}
		if legacyCount > 0 {
			slog.Default().Warn("truncating legacy SofaScore events",
				slog.Int64("count", legacyCount),
			)
			for _, table := range []string{"events", "teams", "tournaments"} {
				if !db.Migrator().HasTable(table) {
					continue
				}
				if err := db.WithContext(ctx).Exec("DELETE FROM " + table).Error; err != nil {
					return fmt.Errorf("truncate %s: %w", table, err)
				}
			}
		}
	}

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
