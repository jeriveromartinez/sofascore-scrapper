// internal/seeder/defaults.go
package seeder

import (
	"fmt"
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper/catalog"
	"gorm.io/gorm"
)

// SeedDefaults runs the first-boot data seed for the FotMob catalog.
// On a fresh database (scraper_leagues empty) it inserts the curated
// initialLeagues slice so the catalog has something to scrape against
// on first run. If the user has already configured leagues (manually
// via the catalog admin endpoints), the seed is a no-op — operators
// retain control and the curated list never overwrites their choices.
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
		var existing int64
		// Fix C2 (PR #122): Unscoped count so soft-deleted rows are
		// counted as "table not empty". The catalog DELETE endpoint
		// uses GORM soft-delete (sets DeletedAt); the underlying row
		// stays in the table and the unique index on source_league_id
		// still applies, so re-inserting the curated seed would
		// collide. Without Unscoped, GORM's default scope returns 0
		// after DELETE-all + restart, SeedDefaults tries to insert,
		// and app.New fails at boot.
		if err := tx.Unscoped().Model(&catalog.ScraperLeague{}).Count(&existing).Error; err != nil {
			return fmt.Errorf("count scraper_leagues: %w", err)
		}
		if existing > 0 {
			return nil
		}
		for i := range initialLeagues {
			if err := tx.Create(&initialLeagues[i]).Error; err != nil {
				return fmt.Errorf("seed scraper_league %s/%s: %w",
					initialLeagues[i].Source, initialLeagues[i].SourceLeagueId, err)
			}
		}
		logger.Info("seeded initial scraper leagues",
			slog.Int("count", len(initialLeagues)),
		)
		return nil
	})
}

// initialLeagues is the curated seed of top world football leagues for
// the FotMob scraper. The first 14 entries match the PR3 brief; the
// remaining entries extend the seed to ~40 so operators have a useful
// out-of-the-box catalog covering the top European leagues, LATAM,
// USA, and the major continental competitions / national cups.
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
	// Rest of Europe.
	{Source: "fotmob", SourceLeagueId: "57", Name: "Eredivisie", Country: "NL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "61", Name: "Primeira Liga", Country: "PT", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "48", Name: "EFL Championship", Country: "GB", Sport: "football", Enabled: true},
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
	// LATAM.
	{Source: "fotmob", SourceLeagueId: "230", Name: "Liga MX", Country: "MX", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "268", Name: "Brasileirão Serie A", Country: "BR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "112", Name: "Superliga Argentina", Country: "AR", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "220", Name: "Primera División Chile", Country: "CL", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "133", Name: "Liga BetPlay Dimayor", Country: "CO", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "139", Name: "Liga 1 Perú", Country: "PE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "240", Name: "LigaPro Ecuador", Country: "EC", Sport: "football", Enabled: true},
	// USA.
	{Source: "fotmob", SourceLeagueId: "130", Name: "Major League Soccer", Country: "US", Sport: "football", Enabled: true},
	// Continental competitions.
	{Source: "fotmob", SourceLeagueId: "42", Name: "Champions League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "73", Name: "Europa League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "8517", Name: "UEFA Europa Conference League", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "113", Name: "Copa Libertadores", Country: "WORLD", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "10216", Name: "Copa Sudamericana", Country: "WORLD", Sport: "football", Enabled: true},
	// National cups.
	{Source: "fotmob", SourceLeagueId: "44", Name: "FA Cup", Country: "GB", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "138", Name: "Copa del Rey", Country: "ES", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "141", Name: "DFB-Pokal", Country: "DE", Sport: "football", Enabled: true},
	{Source: "fotmob", SourceLeagueId: "131", Name: "Copa Argentina", Country: "AR", Sport: "football", Enabled: true},
}
