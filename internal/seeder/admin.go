package seeder

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/auth"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// DefaultAdminEmail is the email seeded on first boot.
	DefaultAdminEmail = "admin@local"

	// DefaultAdminPasswordEnvVar is the environment variable that holds
	// the password of the seeded admin account. When it is set, its
	// value is used; otherwise a random password is generated.
	DefaultAdminPasswordEnvVar = "DEFAULT_ADMIN_PASSWORD"

	// defaultAdminBcryptCost matches auth.BcryptCost so the seeded
	// admin password lands at the current cost without a lazy rehash
	// on first login.
	defaultAdminBcryptCost = auth.BcryptCost
)

// DefaultAdminPassword holds the plaintext password of the seeded
// admin account once SeedDefaultAdmin ran. It is empty before the
// first seed. The value must never be logged or returned to a client.
var DefaultAdminPassword string

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
// command-line path. The password of the seeded account comes from the
// DEFAULT_ADMIN_PASSWORD environment variable; when it is not set a
// random password is generated. See docs/operations/runbook.md for
// changing the default password.
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
	password, generated, err := resolveAdminPassword()
	if err != nil {
		return fmt.Errorf("resolve default admin password: %w", err)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), defaultAdminBcryptCost)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}
	// Backends booting together all see an empty users table; the
	// unique email lets exactly one insert win and the rest no-op
	// instead of failing on the duplicate key and exiting.
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&users.User{
		Email:    DefaultAdminEmail,
		Password: string(hashed),
		Role:     users.RoleAdmin,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		slog.Default().Info("seeder: default admin already exists, skipping",
			slog.String("email", DefaultAdminEmail))
		return nil
	}
	DefaultAdminPassword = password
	if generated {
		slog.Default().Warn("seeder: no admin password configured, a random one was generated for the default admin account. Set "+DefaultAdminPasswordEnvVar+" and recreate the account, or reset the password from the admin API. The generated password is not logged.",
			slog.String("email", DefaultAdminEmail))
	}
	return nil
}

// resolveAdminPassword returns the password of the seeded admin
// account. It comes from the DEFAULT_ADMIN_PASSWORD environment
// variable when set; otherwise a random password is generated and the
// second return value reports it, so the caller can warn the
// operator. The generated password is never logged.
func resolveAdminPassword() (string, bool, error) {
	if fromEnv := strings.TrimSpace(os.Getenv(DefaultAdminPasswordEnvVar)); fromEnv != "" {
		return fromEnv, false, nil
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), true, nil
}
