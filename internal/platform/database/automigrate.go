package database

import (
	"fmt"
	"reflect"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/push"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
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
	&push.DeliveryAttempt{},
}

// AutoMigrateAll runs db.AutoMigrate over every model in
// automigrateModels in order. It is idempotent: GORM does not drop
// or alter existing tables, so it is safe to call on every boot.
//
// If a model is missing from the list it is silently skipped. If a
// foreign key references a model not yet created, GORM errors with
// the model name and AutoMigrateAll returns that error.
func AutoMigrateAll(db *gorm.DB) error {
	for i, model := range automigrateModels {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("automigrate %s (index %d): %w",
				reflect.TypeOf(model).Elem().Name(), i, err)
		}
	}
	return nil
}
