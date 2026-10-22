package database

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/auth"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/playback"
	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
	"github.com/jeriveromartinez/scores365-scrapper/internal/reporting"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper/catalog"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

// automigrateModels is the explicit, topologically-sorted list of
// models AutoMigrateAll will create. The order is mandatory: models
// with foreign keys MUST appear after the models they reference.
//
// When you add a new model with FKs, append it in the correct
// position. When you add a new model without FKs, append it at the
// end. Never reorder existing entries without checking the FK graph.
//
// Source of truth: docs/superpowers/specs/2026-09-01-gorm-automigrate-seeder-design.md §3.2.
var automigrateModels = []any{
	// Lote 0 — independent
	&reporting.CrashReport{},

	// Lote 1 — depend on users
	&users.User{},
	&auth.RefreshToken{},
	&domains.Domain{},
	&apk.ApkVersion{},
	&apk.UploadPublication{},
	&tournaments.Tournament{},
	&tournaments.GlobalTournamentConfig{},
	&reporting.ContentStat{},
	&playback.PlaybackLog{},

	// Lote 2 — depend on lote 1
	&devices.Device{},
	&tournaments.DeviceTournament{},
	&events.Team{},
	&events.Event{},

	// Lote 3 — push, depend on users and devices
	&push.PushMessage{},
	&push.PushMessageTarget{},
	&push.ScheduledPush{},
	&push.ScheduledPushTarget{},
	&push.ScheduledPushTimer{},
	&push.DeliveryAttempt{},

	// Lote 4 — scraper catalog (no FKs)
	&catalog.ScraperLeague{},
}

// AutoMigrateAll runs db.AutoMigrate over every model in
// automigrateModels in order. It is idempotent: GORM does not drop
// or alter existing tables, so it is safe to call on every boot.
//
// If a model is missing from the list it is silently skipped. If a
// foreign key references a model not yet created, GORM errors with
// the model name and AutoMigrateAll returns that error.
//
// AutoMigrateAll also runs PreMigrateLegacyEvents before the
// AutoMigrate loop so legacy events rows get a non-empty
// external_match_id before the unique index is created. See that
// helper's docstring for the rationale.
func AutoMigrateAll(db *gorm.DB) error {
	if err := PreMigrateLegacyEvents(db); err != nil {
		return fmt.Errorf("pre-migrate legacy events: %w", err)
	}
	for i, model := range automigrateModels {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("automigrate %s (index %d): %w",
				reflect.TypeOf(model).Elem().Name(), i, err)
		}
	}
	// The catalog's unique index cannot be installed by AutoMigrate:
	// a legacy deployment already carries an index of the same name
	// covering source_league_id alone, and AutoMigrate never replaces
	// an index that exists. The migration swaps it for the composite
	// one, so it has to run after the table exists.
	if err := catalog.MigrateSourceLeagueIndex(db); err != nil {
		return fmt.Errorf("migrate catalog source league index: %w", err)
	}
	// The FotMob id shift left the tournaments table with a duplicate
	// row per league the pre-shift code path had written. The repair
	// needs the tournaments, events, device assignment and global
	// config tables, so it runs after the AutoMigrate loop.
	if err := MigrateFotMobTournamentDuplicates(db); err != nil {
		return fmt.Errorf("migrate fotmob tournament duplicates: %w", err)
	}
	return nil
}

// PreMigrateLegacyEvents backfills empty external_match_id values on
// legacy events rows so the AutoMigrate that adds the unique index
// idx_events_external_match_id can succeed.
//
// Code review on PR #118 (model reset, merged) flagged this race:
// when the FotMob reset ships to a deployment that pre-existed with
// events rows, the new not-null string column lands as "" for every
// row, and the subsequent unique-index creation fails with a
// duplicate-key error because every "" collides with every other "".
//
// The helper only touches rows where external_match_id is the empty
// string or NULL. Fresh rows (already populated) are left alone.
//
// Deployments that predate the FotMob reset have the events table but
// not the external_match_id column, so the helper adds the column
// first; without it the backfill below fails with "Unknown column"
// and the boot aborts.
//
// It is safe to call before the events table is AutoMigrated; it is
// also a no-op on an empty database. AutoMigrateAll calls this helper
// before the AutoMigrate loop so the contract is "always safe to call
// at boot".
func PreMigrateLegacyEvents(db *gorm.DB) error {
	if db == nil {
		return errors.New("pre-migrate legacy events: nil db")
	}
	if !db.Migrator().HasTable(&events.Event{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&events.Event{}, "external_match_id") {
		if err := db.Migrator().AddColumn(&legacyEventColumn{}, "external_match_id"); err != nil {
			return fmt.Errorf("add legacy external_match_id column: %w", err)
		}
	}
	// id is the primary key and is non-null, so the result is always a
	// non-empty string.
	if err := db.Model(&events.Event{}).
		Unscoped().
		Where("external_match_id = ? OR external_match_id IS NULL", "").
		Update("external_match_id", gorm.Expr(legacyExternalMatchIDExpr(db.Dialector.Name()))).Error; err != nil {
		return fmt.Errorf("backfill legacy external_match_id: %w", err)
	}
	return nil
}

// legacyEventColumn is a minimal projection of the events table used
// to add the external_match_id column to deployments that predate it.
// The field is deliberately nullable: SQLite refuses to add a NOT NULL
// column to a table that already holds rows, and the AutoMigrate that
// runs afterwards tightens the column to NOT NULL.
type legacyEventColumn struct {
	ID              uint    `gorm:"column:id;primaryKey"`
	ExternalMatchID *string `gorm:"column:external_match_id;size:64"`
}

func (legacyEventColumn) TableName() string { return "events" }

// legacyExternalMatchIDExpr returns the SQL expression that prefixes
// the row id with "legacy-". MySQL and MariaDB parse "||" as logical
// OR unless PIPES_AS_CONCAT is part of sql_mode — which would give
// every row the same value and break the unique index — so those
// dialects need CONCAT(). SQLite and Postgres use "||" for string
// concatenation.
func legacyExternalMatchIDExpr(dialect string) string {
	if dialect == "mysql" {
		return "CONCAT('legacy-', id)"
	}
	return "'legacy-' || id"
}
