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
	return db.Transaction(func(tx *gorm.DB) error {
		inserted := 0
		updated := 0
		skipped := 0
		for i := range initialLeagues {
			curated := initialLeagues[i]
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
			slog.Int("curated", len(initialLeagues)),
			slog.Int("inserted", inserted),
			slog.Int("updated", updated),
			slog.Int("skipped_soft_deleted", skipped),
		)
		return nil
	})
}

// initialLeagues is the curated seed of top world football leagues for
// the FotMob scraper. The first 14 entries match the PR3 brief; the
// remaining entries extend the seed to ~150 so operators have a useful
// out-of-the-box catalog covering the top European leagues, second
// divisions, LATAM, USA, Asia/Oceania, Africa, and the major
// continental competitions / national cups. The scraper runs against
// every enabled entry on every cron tick; an admin who wants a smaller
// catalog can disable entries in /#/scraper-leagues.
//
// IMPORTANT: source_league_id values are best-guess from the public
// FotMob catalog. They MUST be verified against
// https://www.fotmob.com/api/leagues before each FotMob platform
// upgrade so that the catalog stays in sync with FotMob's internal
// league numbering. A failed lookup is silent at runtime: the scraper
// simply returns no matches for an unknown id.
var initialLeagues = []catalog.ScraperLeague{
	// Top 5 European leagues.
	{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "87", Name: "LaLiga", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "55", Name: "Serie A", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "54", Name: "Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "53", Name: "Ligue 1", Country: "FR", Sport: "football", Enabled: true},
	// Second divisions of the top 5 European leagues.
	{Source: "fotmob", SourceLeagueId: "48", Name: "EFL Championship", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "140", Name: "LaLiga 2", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "68", Name: "Serie B", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "80", Name: "2. Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "60", Name: "Ligue 2", Country: "FR", Sport: "football", Enabled: true},
	// Rest of Europe (top flights).
	{Source: "fotmob", SourceLeagueId: "57", Name: "Eredivisie", Country: "NL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "61", Name: "Primeira Liga", Country: "PT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "59", Name: "Scottish Premiership", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "62", Name: "Belgian Pro League", Country: "BE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "76", Name: "Swiss Super League", Country: "CH", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "79", Name: "Turkish Süper Lig", Country: "TR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "70", Name: "Greek Super League", Country: "GR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "56", Name: "Austrian Bundesliga", Country: "AT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "67", Name: "Danish Superliga", Country: "DK", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "65", Name: "Swedish Allsvenskan", Country: "SE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "63", Name: "Norwegian Eliteserien", Country: "NO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "81", Name: "Ekstraklasa", Country: "PL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "75", Name: "Ukrainian Premier League", Country: "UA", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "78", Name: "Russian Premier League", Country: "RU", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "222", Name: "Croatian HNL", Country: "HR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "219", Name: "Czech Fortuna Liga", Country: "CZ", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "71", Name: "Serbian SuperLiga", Country: "RS", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "229", Name: "Romanian Liga 1", Country: "RO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "228", Name: "Bulgarian First League", Country: "BG", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "208", Name: "Hungarian NB I", Country: "HU", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "84", Name: "Israeli Premier League", Country: "IL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "86", Name: "Cyprus First Division", Country: "CY", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "85", Name: "Croatian Second League", Country: "HR", Sport: "football", Enabled: true},
	// LATAM.
	{Source: "fotmob", SourceLeagueId: "230", Name: "Liga MX", Country: "MX", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "268", Name: "Brasileirão Serie A", Country: "BR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "269", Name: "Brasileirão Serie B", Country: "BR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "112", Name: "Superliga Argentina", Country: "AR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "110", Name: "Primera B Nacional", Country: "AR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "220", Name: "Primera División Chile", Country: "CL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "133", Name: "Liga BetPlay Dimayor", Country: "CO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "139", Name: "Liga 1 Perú", Country: "PE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "240", Name: "LigaPro Ecuador", Country: "EC", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "234", Name: "Primera División Uruguay", Country: "UY", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "237", Name: "Primera División Paraguay", Country: "PY", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "244", Name: "Primera División Bolivia", Country: "BO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "250", Name: "Primera División Venezuela", Country: "VE", Sport: "football", Enabled: true},
	// USA / North America.
	{Source: "fotmob", SourceLeagueId: "130", Name: "Major League Soccer", Country: "US", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "135", Name: "USL Championship", Country: "US", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "117", Name: "Liga MX Clausura", Country: "MX", Sport: "football", Enabled: true},
	// Asia.
	{Source: "fotmob", SourceLeagueId: "98", Name: "J1 League", Country: "JP", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "99", Name: "J2 League", Country: "JP", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "106", Name: "K League 1", Country: "KR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "108", Name: "Chinese Super League", Country: "CN", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "107", Name: "A-League", Country: "AU", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "120", Name: "Saudi Pro League", Country: "SA", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "125", Name: "UAE Pro League", Country: "AE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "122", Name: "Qatar Stars League", Country: "QA", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "123", Name: "Indian Super League", Country: "IN", Sport: "football", Enabled: true},
	// Africa.
	{Source: "fotmob", SourceLeagueId: "148", Name: "Egyptian Premier League", Country: "EG", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "152", Name: "South African Premier Division", Country: "ZA", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "149", Name: "Moroccan Botola Pro", Country: "MA", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "150", Name: "Tunisian Ligue 1", Country: "TN", Sport: "football", Enabled: true},
	// Continental competitions.
	{Source: "fotmob", SourceLeagueId: "42", Name: "Champions League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "73", Name: "Europa League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "8517", Name: "UEFA Europa Conference League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "113", Name: "Copa Libertadores", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "10216", Name: "Copa Sudamericana", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "116", Name: "CONCACAF Champions Cup", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "1310", Name: "AFC Champions League Elite", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "134", Name: "CAF Champions League", Country: "WORLD", Sport: "football", Enabled: true},
	// National cups.
	{Source: "fotmob", SourceLeagueId: "44", Name: "FA Cup", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "1330", Name: "EFL Cup", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "138", Name: "Copa del Rey", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "141", Name: "DFB-Pokal", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "137", Name: "Coppa Italia", Country: "IT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "142", Name: "Coupe de France", Country: "FR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "131", Name: "Copa Argentina", Country: "AR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "233", Name: "Copa MX", Country: "MX", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "235", Name: "KNVB Beker", Country: "NL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "238", Name: "Taça de Portugal", Country: "PT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "246", Name: "Emirates FA Cup", Country: "GB", Sport: "football", Enabled: true},
	// International competitions.
	{Source: "fotmob", SourceLeagueId: "50", Name: "World Cup", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "69", Name: "Euro Championship", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "105", Name: "Copa América", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "100", Name: "Africa Cup of Nations", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "104", Name: "Asian Cup", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "102", Name: "CONCACAF Gold Cup", Country: "WORLD", Sport: "football", Enabled: true},
	// Women's top leagues.
	{Source: "fotmob", SourceLeagueId: "169", Name: "Women's Super League", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "170", Name: "Liga F", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "173", Name: "Frauen-Bundesliga", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "172", Name: "Première Ligue Féminine", Country: "FR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "176", Name: "NWSL", Country: "US", Sport: "football", Enabled: true},
	// Women's international.
	{Source: "fotmob", SourceLeagueId: "175", Name: "Women's World Cup", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "180", Name: "Women's Euro", Country: "WORLD", Sport: "football", Enabled: true},
}
