// Package seeder provides first-boot data for a fresh database.
package seeder

import (
	"context"
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
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

// SeedDefaults runs the first-boot seed under a single transaction.
// On the first boot after the FotMob model reset, it detects legacy
// SofaScore events (rows with source = "sofascore" or with the empty
// default source) and truncates events/teams/tournaments before
// continuing with the rest of the default seed (admin user, etc.).
//
// The guard is idempotent: once the legacy rows are gone, subsequent
// boots are no-ops for the truncation branch. A nil logger falls back
// to slog.Default() so callers can opt out of structured logging
// during tests.
func SeedDefaults(db *gorm.DB, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var legacyCount int64
		if err := tx.Model(&events.Event{}).
			Where("source = ? OR source = ''", "sofascore").
			Count(&legacyCount).Error; err != nil {
			return err
		}
		if legacyCount > 0 {
			logger.Warn("truncating legacy SofaScore events",
				slog.Int64("count", legacyCount),
			)
			if err := tx.Exec("DELETE FROM events").Error; err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM teams").Error; err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM tournaments").Error; err != nil {
				return err
			}
		}
		return SeedDefaultAdmin(context.Background(), tx)
	})
}
