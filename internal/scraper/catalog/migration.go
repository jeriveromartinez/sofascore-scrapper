// internal/scraper/catalog/migration.go
package catalog

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// sourceLeagueIndexName is the name of the unique index that guards
// the catalog against duplicate leagues. It used to cover
// source_league_id alone, which made the same upstream id unusable at
// two sources; it now covers (source, source_league_id).
const sourceLeagueIndexName = "idx_scraper_leagues_source_league"

// MigrateSourceLeagueIndex replaces the legacy single-column unique
// index with the composite one.
//
// GORM's AutoMigrate never replaces an index that already exists under
// the same name, so the legacy index has to be dropped explicitly
// before the composite one can be created. Rows that would violate
// the composite key are removed first: the oldest row of every
// (source, source_league_id) group survives, the rest are hard
// deleted (a soft delete would keep the unique key occupied).
//
// It is idempotent and safe to run on every boot, including when
// another backend migrates the same database at the same time: the
// index is classified again right before it is dropped, so a backend
// that lost the snapshot race does not drop the composite index
// another backend just created. Backends that take the migration lock
// are fully serialized; the re-read narrows the window for an older
// backend that does not take it.
func MigrateSourceLeagueIndex(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	m := db.Migrator()
	if !m.HasTable(&ScraperLeague{}) {
		return nil
	}
	state, err := sourceLeagueIndexState(m)
	if err != nil {
		return err
	}
	if state == sourceLeagueIndexComposite {
		return nil
	}
	if state == sourceLeagueIndexLegacy {
		if err := dropLegacySourceLeagueIndex(m); err != nil {
			return err
		}
	}
	if err := dedupeSourceLeagueID(db); err != nil {
		return err
	}
	return createCompositeSourceLeagueIndex(m)
}

type sourceLeagueIndexStatus int

const (
	sourceLeagueIndexMissing sourceLeagueIndexStatus = iota
	sourceLeagueIndexLegacy
	sourceLeagueIndexComposite
)

// sourceLeagueIndexMigrator is the slice of the GORM migrator the
// index swap needs; gorm.Migrator satisfies it.
type sourceLeagueIndexMigrator interface {
	GetIndexes(value any) ([]gorm.Index, error)
	DropIndex(value any, name string) error
	CreateIndex(value any, name string) error
}

func sourceLeagueIndexState(m sourceLeagueIndexMigrator) (sourceLeagueIndexStatus, error) {
	indexes, err := m.GetIndexes(&ScraperLeague{})
	if err != nil {
		return sourceLeagueIndexMissing, err
	}
	for _, idx := range indexes {
		if idx.Name() != sourceLeagueIndexName {
			continue
		}
		if isCompositeSourceLeagueIndex(idx) {
			return sourceLeagueIndexComposite, nil
		}
		return sourceLeagueIndexLegacy, nil
	}
	return sourceLeagueIndexMissing, nil
}

// isCompositeSourceLeagueIndex accepts only the unique index the model
// declares: a same-named index with the right columns but without the
// uniqueness leaves the table unguarded and has to be replaced too.
func isCompositeSourceLeagueIndex(idx gorm.Index) bool {
	cols := idx.Columns()
	if len(cols) != 2 || cols[0] != "source" || cols[1] != "source_league_id" {
		return false
	}
	unique, ok := idx.Unique()
	return !ok || unique
}

// dropLegacySourceLeagueIndex reads the index again right before the
// drop: the caller's snapshot may be stale, and DropIndex matches by
// name, so dropping on the old snapshot could remove the composite
// index another backend created in the meantime.
func dropLegacySourceLeagueIndex(m sourceLeagueIndexMigrator) error {
	state, err := sourceLeagueIndexState(m)
	if err != nil {
		return err
	}
	if state != sourceLeagueIndexLegacy {
		return nil
	}
	if err := m.DropIndex(&ScraperLeague{}, sourceLeagueIndexName); err != nil {
		return fmt.Errorf("drop legacy source league index: %w", err)
	}
	return nil
}

// createCompositeSourceLeagueIndex installs the composite index and
// verifies it landed. The second pass covers another backend that
// dropped the index between the dedupe and the create.
func createCompositeSourceLeagueIndex(m sourceLeagueIndexMigrator) error {
	for range 2 {
		state, err := sourceLeagueIndexState(m)
		if err != nil {
			return err
		}
		switch state {
		case sourceLeagueIndexComposite:
			return nil
		case sourceLeagueIndexLegacy:
			if err := dropLegacySourceLeagueIndex(m); err != nil {
				return err
			}
		case sourceLeagueIndexMissing:
			if err := m.CreateIndex(&ScraperLeague{}, sourceLeagueIndexName); err != nil {
				return fmt.Errorf("create source league index: %w", err)
			}
		}
	}
	state, err := sourceLeagueIndexState(m)
	if err != nil {
		return err
	}
	if state != sourceLeagueIndexComposite {
		return fmt.Errorf("source league index %q is not composite after creation", sourceLeagueIndexName)
	}
	return nil
}

// dedupeSourceLeagueID removes the rows that share a
// (source, source_league_id) pair, keeping the oldest one.
func dedupeSourceLeagueID(db *gorm.DB) error {
	type duplicate struct {
		Source         string
		SourceLeagueId string
	}
	var duplicates []duplicate
	if err := db.Raw(
		"SELECT source, source_league_id FROM scraper_leagues " +
			"GROUP BY source, source_league_id HAVING COUNT(*) > 1",
	).Scan(&duplicates).Error; err != nil {
		return err
	}
	for _, dup := range duplicates {
		var keeper struct {
			ID uint
		}
		if err := db.Raw(
			"SELECT MIN(id) AS id FROM scraper_leagues WHERE source = ? AND source_league_id = ?",
			dup.Source, dup.SourceLeagueId,
		).Scan(&keeper).Error; err != nil {
			return err
		}
		if keeper.ID == 0 {
			continue
		}
		res := db.Unscoped().
			Where("source = ? AND source_league_id = ? AND id > ?",
				dup.Source, dup.SourceLeagueId, keeper.ID).
			Delete(&ScraperLeague{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			slog.Default().Warn("catalog: removed duplicate scraper league rows",
				slog.String("source", dup.Source),
				slog.String("source_league_id", dup.SourceLeagueId),
				slog.Int64("removed", res.RowsAffected))
		}
	}
	return nil
}
