package database

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper/catalog"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

// fotMobSource is the value the scraper catalog stores in the source
// column for the FotMob leagues.
const fotMobSource = "fotmob"

// tournamentIdentity is the subset of a tournaments row the migration
// compares to decide whether two rows describe the same tournament.
type tournamentIdentity struct {
	ID     uint
	Name   string
	Region string
}

// MigrateFotMobTournamentDuplicates repairs the tournaments rows that
// the FotMob id namespace shift left behind.
//
// The scraper used to store a FotMob league under its raw upstream
// competition id; it now stores it under FotMobTournamentIDBase + id.
// A deployment that ran the old code path therefore holds the same
// real-world league twice: once under the raw id (legacy) and once
// under the shifted id (canonical). Every event written after the
// shift points at the shifted id, so the legacy row is an orphan that
// only shows up as a duplicate in the admin tournament list.
//
// For every legacy row whose shifted twin exists the migration moves
// every reference (events, device assignments, global configs) to the
// shifted row and then soft-deletes the legacy one. The soft delete
// matches what tournaments.Repository.Delete does, keeps the row for
// forensics and leaves the primary key free of a live row.
//
// A legacy row whose name matches neither the shifted twin nor the
// scraper catalog is left alone and logged: it is most likely an
// admin-created tournament that happened to land on an upstream id.
//
// The migration is idempotent — a soft-deleted legacy row is not a
// candidate on the next run — and a no-op on a database without
// duplicates.
func MigrateFotMobTournamentDuplicates(db *gorm.DB) error {
	if db == nil {
		return errors.New("migrate fotmob tournament duplicates: nil db")
	}
	if !db.Migrator().HasTable(&tournaments.Tournament{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		repaired, skipped, err := repairLegacyFotMobTournaments(tx)
		if err != nil {
			return err
		}
		if repaired > 0 || skipped > 0 {
			slog.Default().Info("database: repaired duplicated FotMob tournaments",
				slog.Int("repaired", repaired),
				slog.Int("skipped", skipped))
		}
		return nil
	})
}

// repairLegacyFotMobTournaments resolves every legacy row that has a
// shifted twin and returns how many were repaired and how many were
// left in place.
func repairLegacyFotMobTournaments(tx *gorm.DB) (repaired int, skipped int, err error) {
	legacy, err := legacyFotMobTournamentRows(tx)
	if err != nil {
		return 0, 0, err
	}
	if len(legacy) == 0 {
		return 0, 0, nil
	}

	twins, err := shiftedFotMobTournamentRows(tx, legacy)
	if err != nil {
		return 0, 0, err
	}
	if len(twins) == 0 {
		return 0, 0, nil
	}

	catalogNames, err := catalogFotMobLeagueNames(tx, legacy)
	if err != nil {
		return 0, 0, err
	}

	for _, row := range legacy {
		twin, ok := twins[row.ID]
		if !ok {
			// The league was never scraped after the shift, so the
			// legacy row is the only one there is: nothing to repair.
			continue
		}
		if !legacyRowIsFotMobLeague(row, twin, catalogNames) {
			skipped++
			slog.Default().Warn("database: left duplicate tournament rows in place",
				slog.Uint64("legacy_id", uint64(row.ID)),
				slog.Uint64("shifted_id", uint64(twin.ID)),
				slog.String("legacy_name", row.Name),
				slog.String("shifted_name", twin.Name))
			continue
		}
		if err := repointTournamentReferences(tx, row.ID, twin.ID); err != nil {
			return repaired, skipped, err
		}
		if err := tx.Delete(&tournaments.Tournament{}, row.ID).Error; err != nil {
			return repaired, skipped, fmt.Errorf("delete legacy tournament %d: %w", row.ID, err)
		}
		repaired++
		slog.Default().Info("database: removed duplicated FotMob tournament",
			slog.Uint64("legacy_id", uint64(row.ID)),
			slog.Uint64("shifted_id", uint64(twin.ID)),
			slog.String("name", row.Name))
	}
	return repaired, skipped, nil
}

// legacyFotMobTournamentRows returns the live tournaments rows that
// the pre-shift code path could have written: rows whose primary key
// is the raw FotMob competition id.
//
// The ids above the admin band belong to the 365scores band (6e9+) or
// to the admin band (1e9..2e9). Neither was ever shifted, so they are
// not candidates.
func legacyFotMobTournamentRows(tx *gorm.DB) ([]tournamentIdentity, error) {
	var rows []tournamentIdentity
	err := tx.Model(&tournaments.Tournament{}).
		Where("id > 0 AND id < ?", uint64(tournaments.AdminTournamentIDBase)).
		Select("id, name, region").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list legacy tournaments: %w", err)
	}
	return rows, nil
}

// shiftedFotMobTournamentRows returns the shifted twin of every legacy
// row that has one, keyed by the legacy id.
func shiftedFotMobTournamentRows(tx *gorm.DB, legacy []tournamentIdentity) (map[uint]tournamentIdentity, error) {
	shiftedIDs := make([]uint, 0, len(legacy))
	for _, row := range legacy {
		shiftedIDs = append(shiftedIDs, uint(uint64(row.ID)+tournaments.FotMobTournamentIDBase))
	}
	twins := make(map[uint]tournamentIdentity, len(legacy))
	err := forEachIDBatch(shiftedIDs, func(batch []uint) error {
		var rows []tournamentIdentity
		if err := tx.Model(&tournaments.Tournament{}).
			Where("id IN ?", batch).
			Select("id, name, region").
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			legacyID := uint(uint64(row.ID) - tournaments.FotMobTournamentIDBase)
			twins[legacyID] = row
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list shifted tournaments: %w", err)
	}
	return twins, nil
}

// catalogFotMobLeagueNames maps the raw FotMob league ids the scraper
// catalog knows about to the league name it publishes. The catalog is
// the only table that records which upstream published an id, so it is
// the independent evidence that a legacy row is a FotMob league and
// not an admin-created tournament.
func catalogFotMobLeagueNames(tx *gorm.DB, legacy []tournamentIdentity) (map[uint]string, error) {
	if !tx.Migrator().HasTable(&catalog.ScraperLeague{}) {
		return nil, nil
	}
	keys := make([]string, 0, len(legacy))
	for _, row := range legacy {
		keys = append(keys, strconv.FormatUint(uint64(row.ID), 10))
	}
	names := make(map[uint]string, len(keys))
	err := forEachIDBatch(keys, func(batch []string) error {
		var rows []struct {
			SourceLeagueID string
			Name           string
		}
		if err := tx.Model(&catalog.ScraperLeague{}).
			Where("LOWER(source) = ? AND source_league_id IN ?", fotMobSource, batch).
			Select("source_league_id, name").
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := strconv.ParseUint(strings.TrimSpace(row.SourceLeagueID), 10, 64)
			if err != nil {
				continue
			}
			names[uint(raw)] = row.Name
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read scraper catalog: %w", err)
	}
	return names, nil
}

// idBatchSize bounds the number of bind parameters a single IN clause
// carries: SQLite caps the variables of a prepared statement.
const idBatchSize = 500

// forEachIDBatch runs fn over the given ids in batches small enough for
// an IN clause.
func forEachIDBatch[T any](ids []T, fn func([]T) error) error {
	for start := 0; start < len(ids); start += idBatchSize {
		end := min(start+idBatchSize, len(ids))
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// legacyRowIsFotMobLeague reports whether the legacy row holds the
// FotMob league whose shifted twin is the given row.
//
// The row matches when it carries the same name as the twin or as the
// catalog entry for the same upstream id. A row that matches neither
// is an admin-created tournament that landed on an upstream id and is
// left untouched.
func legacyRowIsFotMobLeague(legacy, twin tournamentIdentity, catalogNames map[uint]string) bool {
	if strings.TrimSpace(legacy.Name) == "" {
		return false
	}
	if sameTournamentName(legacy.Name, twin.Name) {
		return true
	}
	name, ok := catalogNames[legacy.ID]
	return ok && sameTournamentName(legacy.Name, name)
}

// sameTournamentName compares two tournament names the way the admin
// list shows them.
func sameTournamentName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// repointTournamentReferences moves every reference to the legacy
// tournament over to the shifted one.
//
// The references are moved before the legacy row is deleted because
// the foreign keys declare ON DELETE CASCADE: deleting the parent
// first would wipe the child rows instead of moving them.
func repointTournamentReferences(tx *gorm.DB, legacyID, shiftedID uint) error {
	m := tx.Migrator()
	if m.HasTable(&events.Event{}) {
		if err := tx.Model(&events.Event{}).Unscoped().
			Where("league_id = ?", legacyID).
			Update("league_id", shiftedID).Error; err != nil {
			return fmt.Errorf("repoint events of tournament %d: %w", legacyID, err)
		}
	}
	if m.HasTable(&tournaments.DeviceTournament{}) {
		if err := repointDeviceAssignments(tx, legacyID, shiftedID); err != nil {
			return err
		}
	}
	if m.HasTable(&tournaments.GlobalTournamentConfig{}) {
		if err := repointGlobalConfigs(tx, legacyID, shiftedID); err != nil {
			return err
		}
	}
	return nil
}

// repointDeviceAssignments moves the device assignments that point at
// the legacy tournament over to the shifted one.
//
// A device can be assigned to both rows, so the two assignments have
// to collapse into one: the unique index on (device_id,
// tournament_id) forbids them side by side.
func repointDeviceAssignments(tx *gorm.DB, legacyID, shiftedID uint) error {
	var assignments []tournaments.DeviceTournament
	if err := tx.Unscoped().
		Where("tournament_id = ?", legacyID).
		Find(&assignments).Error; err != nil {
		return fmt.Errorf("list device assignments of tournament %d: %w", legacyID, err)
	}
	for _, assignment := range assignments {
		if assignment.DeletedAt.Valid {
			// The device was unassigned before the migration; the row
			// only occupies the unique index slot of an id nothing
			// writes again.
			continue
		}
		shifted, err := deviceAssignmentsFor(tx, assignment.DeviceID, shiftedID)
		if err != nil {
			return err
		}
		live := false
		for _, row := range shifted {
			if row.DeletedAt.Valid {
				// A soft-deleted row still occupies the unique index
				// slot, so it has to go before the assignment moves.
				if err := tx.Unscoped().Delete(&tournaments.DeviceTournament{}, row.ID).Error; err != nil {
					return fmt.Errorf("drop stale device assignment %d: %w", row.ID, err)
				}
				continue
			}
			live = true
		}
		if live {
			// The device is already assigned to the shifted tournament.
			if err := tx.Unscoped().Delete(&tournaments.DeviceTournament{}, assignment.ID).Error; err != nil {
				return fmt.Errorf("drop duplicated device assignment %d: %w", assignment.ID, err)
			}
			continue
		}
		if err := tx.Unscoped().Model(&tournaments.DeviceTournament{}).
			Where("id = ?", assignment.ID).
			Update("tournament_id", shiftedID).Error; err != nil {
			return fmt.Errorf("repoint device assignment %d: %w", assignment.ID, err)
		}
	}
	return nil
}

// deviceAssignmentsFor returns every assignment row of a device for a
// tournament, soft-deleted ones included.
func deviceAssignmentsFor(tx *gorm.DB, deviceID, tournamentID uint) ([]tournaments.DeviceTournament, error) {
	var rows []tournaments.DeviceTournament
	if err := tx.Unscoped().
		Where("device_id = ? AND tournament_id = ?", deviceID, tournamentID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list device assignments of tournament %d: %w", tournamentID, err)
	}
	return rows, nil
}

// repointGlobalConfigs moves the global tournament configuration over
// to the shifted tournament.
//
// The table holds one row per tournament (unique index on
// tournament_id) and nothing else, so a config that already exists
// for the shifted tournament makes the legacy one redundant.
func repointGlobalConfigs(tx *gorm.DB, legacyID, shiftedID uint) error {
	var configs []tournaments.GlobalTournamentConfig
	if err := tx.Unscoped().
		Where("tournament_id = ?", legacyID).
		Find(&configs).Error; err != nil {
		return fmt.Errorf("list global configs of tournament %d: %w", legacyID, err)
	}
	for _, config := range configs {
		if config.DeletedAt.Valid {
			continue
		}
		shifted, err := globalConfigsFor(tx, shiftedID)
		if err != nil {
			return err
		}
		live := false
		for _, row := range shifted {
			if row.DeletedAt.Valid {
				if err := tx.Unscoped().Delete(&tournaments.GlobalTournamentConfig{}, row.ID).Error; err != nil {
					return fmt.Errorf("drop stale global config %d: %w", row.ID, err)
				}
				continue
			}
			live = true
		}
		if live {
			if err := tx.Unscoped().Delete(&tournaments.GlobalTournamentConfig{}, config.ID).Error; err != nil {
				return fmt.Errorf("drop duplicated global config %d: %w", config.ID, err)
			}
			continue
		}
		if err := tx.Unscoped().Model(&tournaments.GlobalTournamentConfig{}).
			Where("id = ?", config.ID).
			Update("tournament_id", shiftedID).Error; err != nil {
			return fmt.Errorf("repoint global config %d: %w", config.ID, err)
		}
	}
	return nil
}

// globalConfigsFor returns every global config row of a tournament,
// soft-deleted ones included.
func globalConfigsFor(tx *gorm.DB, tournamentID uint) ([]tournaments.GlobalTournamentConfig, error) {
	var rows []tournaments.GlobalTournamentConfig
	if err := tx.Unscoped().
		Where("tournament_id = ?", tournamentID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list global configs of tournament %d: %w", tournamentID, err)
	}
	return rows, nil
}
