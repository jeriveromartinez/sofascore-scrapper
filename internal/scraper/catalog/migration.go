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
// It is idempotent and safe to run on every boot.
func MigrateSourceLeagueIndex(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	m := db.Migrator()
	if !m.HasTable(&ScraperLeague{}) {
		return nil
	}
	indexes, err := m.GetIndexes(&ScraperLeague{})
	if err != nil {
		return err
	}
	for _, idx := range indexes {
		if idx.Name() != sourceLeagueIndexName {
			continue
		}
		cols := idx.Columns()
		if len(cols) == 2 && cols[0] == "source" && cols[1] == "source_league_id" {
			return nil
		}
		if err := m.DropIndex(&ScraperLeague{}, sourceLeagueIndexName); err != nil {
			return fmt.Errorf("drop legacy source league index: %w", err)
		}
	}
	if err := dedupeSourceLeagueID(db); err != nil {
		return err
	}
	if m.HasIndex(&ScraperLeague{}, sourceLeagueIndexName) {
		return nil
	}
	if err := m.CreateIndex(&ScraperLeague{}, sourceLeagueIndexName); err != nil {
		return fmt.Errorf("create source league index: %w", err)
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
