// internal/seeder/defaults.go
package seeder

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper/catalog"
	"gorm.io/gorm"
)

// SeedDefaults reconciles the scraper_leagues table with the curated
// initialLeagues slice on every boot. For each curated row:
//
//   - If no row exists with that source_league_id, the curated row
//     is inserted.
//   - If a row exists with that source_league_id and is NOT
//     soft-deleted, its name/country/sport columns are rewritten to
//     the curated values (so deployments running an older seed pick
//     up ID corrections and FotMob name updates). The `enabled` flag
//     is left untouched — operators who disabled a curated league keep
//     that choice across boots.
//   - If a row exists with that source_league_id but IS soft-deleted,
//     the curated row is skipped. The operator's intent ("do not
//     scrape this league") is preserved across boots.
//
// Rows whose source_league_id is not in the curated slice (e.g.
// operator-added leagues via /#/scraper-leagues or
// /scraper-leagues/search) are left untouched.
//
// Note on FotMob ID changes: if the curated seed renames the id for a
// league (e.g. an upstream FotMob id change), the OLD row stays in
// the table under its previous id. Operators see two rows in
// /#/scraper-leagues and can disable the old one manually. See
// docs/operations/runbook.md for the cleanup SQL.
//
// The seed runs inside a single transaction so partial inserts cannot
// leave the table in a half-populated state. If the scraper_leagues
// table has not been migrated yet the seed will surface that as an
// error; callers are expected to AutoMigrate the catalog first.
//
// Logging goes through the injected logger when non-nil, falling back
// to slog.Default(). This mirrors the convention used by
// SeedDefaultAdmin (which always uses slog.Default()).
//
// Called automatically from app.New on every boot when SKIP_MIGRATE is
// not set, alongside SeedDefaultAdmin. See docs/operations/runbook.md
// for adjusting the curated list.
func SeedDefaults(db *gorm.DB, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	all := append([]catalog.ScraperLeague{}, initialLeagues...)

	return db.Transaction(func(tx *gorm.DB) error {
		inserted := 0
		updated := 0
		skipped := 0
		for i := range all {
			curated := all[i]
			var existing catalog.ScraperLeague
			err := tx.Unscoped().
				Where("source_league_id = ?", curated.SourceLeagueId).
				First(&existing).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				// Brand-new curated row: insert.
				if createErr := tx.Create(&curated).Error; createErr != nil {
					return fmt.Errorf("seed scraper_league %s/%s: %w",
						curated.Source, curated.SourceLeagueId, createErr)
				}
				inserted++
			case err != nil:
				return fmt.Errorf("lookup scraper_league %s/%s: %w",
					curated.Source, curated.SourceLeagueId, err)
			case existing.DeletedAt.Valid:
				// Operator soft-deleted this curated entry — honor
				// the deletion across boots; skip the reconciliation.
				skipped++
			default:
				// Existing live row: rewrite name/country/sport to
				// the curated values. DO NOT touch `enabled` — see
				// comment above.
				if updateErr := tx.Unscoped().Model(&existing).
					Updates(map[string]any{
						"name":    curated.Name,
						"country": curated.Country,
						"sport":   curated.Sport,
					}).Error; updateErr != nil {
					return fmt.Errorf("update scraper_league %s/%s: %w",
							curated.Source, curated.SourceLeagueId, updateErr)
				}
				updated++
			}
		}
		logger.Info("reconciled scraper leagues with curated seed",
			slog.Int("curated", len(all)),
			slog.Int("inserted", inserted),
			slog.Int("updated", updated),
			slog.Int("skipped_soft_deleted", skipped),
		)
		return nil
	})
}

// initialLeagues is the curated seed of FotMob leagues that the
// scheduler scrapes every cron tick. Each entry's source_league_id
// was verified against the real FotMob
// `https://www.fotmob.com/api/data/matches?date=…&timezone=Europe/Paris`
// endpoint on 2026-09-13: if a league with this id shows up in the
// daily payload, the seed row uses the upstream `name`/`ccode` we
// observed (with `country` normalised to an ISO-2 letter code).
//
// Entries that did NOT verify against the real catalog are
// deliberately omitted — FotMob assigns many leagues with similar
// human-readable names (e.g. "Premier League" exists for the UK, the
// Norwegian top flight, the Venezuelan top flight, etc.) and a wrong
// id is silently a no-op at scrape time. Admins who want leagues that
// are not in this seed should call `GET /api/web/v1/scraper-leagues/search?q=…`
// (implemented against FotMob's /api/searchapi/suggest endpoint) and
// POST the result back via `/api/web/v1/scraper-leagues`.
var initialLeagues = []catalog.ScraperLeague{
	// Top 5 European leagues (verified).
	{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "87", Name: "LaLiga", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "55", Name: "Serie A", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "54", Name: "Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "53", Name: "Ligue 1", Country: "FR", Sport: "football", Enabled: true},
	// Second divisions of the top 5 (verified).
	{Source: "fotmob", SourceLeagueId: "938218", Name: "EFL Championship", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "938653", Name: "LaLiga2", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "941726", Name: "Serie B", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "146", Name: "2. Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "110", Name: "Ligue 2", Country: "FR", Sport: "football", Enabled: true},
	// Rest of Europe top flights (verified).
	{Source: "fotmob", SourceLeagueId: "937276", Name: "Eredivisie", Country: "NL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "61", Name: "Liga Portugal", Country: "PT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "59", Name: "Eliteserien", Country: "NO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "67", Name: "Allsvenskan", Country: "SE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "935487", Name: "Superligaen", Country: "DK", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "71", Name: "Super Lig", Country: "TR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "937988", Name: "Belgian Pro League", Country: "BE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "937879", Name: "Ekstraklasa", Country: "PL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "8983", Name: "Super League", Country: "CH", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "938366", Name: "Bundesliga", Country: "AT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "252", Name: "HNL", Country: "HR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "212", Name: "NB I", Country: "HU", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "942063", Name: "Cyprus League", Country: "CY", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "127", Name: "Ligat ha'Al", Country: "IL", Sport: "football", Enabled: true},
	// LATAM (verified).
	{Source: "fotmob", SourceLeagueId: "230", Name: "Liga MX", Country: "MX", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "268", Name: "Série A", Country: "BR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "8814", Name: "Série B", Country: "BR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "131", Name: "Liga 1", Country: "AR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "916553", Name: "Primera B Metropolitana", Country: "AR", Sport: "football", Enabled: true},
	// USA / North America (verified).
	{Source: "fotmob", SourceLeagueId: "913550", Name: "Major League Soccer", Country: "US", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "8972", Name: "USL Championship", Country: "US", Sport: "football", Enabled: true},
	// Asia (verified).
	{Source: "fotmob", SourceLeagueId: "937875", Name: "J. League", Country: "JP", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "919356", Name: "K-League 1", Country: "KR", Sport: "football", Enabled: true},
	// National cups (verified).
	{Source: "fotmob", SourceLeagueId: "177", Name: "FA Cup", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "938221", Name: "EFL Cup", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "938423", Name: "Coppa Italia", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "206", Name: "Cup", Country: "FR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "9305", Name: "Copa Argentina", Country: "AR", Sport: "football", Enabled: true},
	// Women's (verified).
	{Source: "fotmob", SourceLeagueId: "9676", Name: "Frauen-Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "938777", Name: "Liga F", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "920228", Name: "NWSL", Country: "US", Sport: "football", Enabled: true},
}
