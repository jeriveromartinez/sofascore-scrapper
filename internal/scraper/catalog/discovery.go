// Package catalog houses the bulk-discovery glue that keeps the
// admin scraper catalog in sync with the upstream
// (TheSportsDB) league registry.
//
// Background: PR #131/132 wired the scraper to ingest events
// for four pre-seeded leagues (NBA/NFL/MLB/NHL). PR #133 added
// the bulk-fetch path (eventsday.php without an `l=` filter)
// so every event TheSportsDB publishes for the day is now
// scraped in one HTTP call. That is great for coverage but
// useless unless the catalog actually knows about every
// league the upstream surfaces — otherwise the dispatch loop
// drops matches whose idLeague is not in `scraper_leagues`.
//
// This package fixes that with a single-shot discovery job
// that pulls the upstream league registry, normalises the
// sport names, and upserts every row into `scraper_leagues`
// with enabled=true. Operators can flip a row off later via
// the existing admin UI.
package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/sportsdb"
)

// DiscoveryResult is the per-run summary logged so operators
// can see what was added on each invocation.
type DiscoveryResult struct {
	Upserted   int // newly inserted rows
	Unchanged  int // rows that already existed (idempotent path)
	Skipped    int // rows the upstream published but with no id
	TotalSeen  int // total rows the upstream returned
	Source     string
}

func (r DiscoveryResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("source", r.Source),
		slog.Int("upserted", r.Upserted),
		slog.Int("unchanged", r.Unchanged),
		slog.Int("skipped", r.Skipped),
		slog.Int("total_seen", r.TotalSeen),
	)
}

// Discovery sweeps the upstream league registry and upserts
// every row into `scraper_leagues`. Idempotent: re-running on
// the same registry is a no-op (the unique key on
// (source, source_league_id) blocks duplicates and EnsureLeague
// skips already-present rows).
//
// Currently this only wires the sportsdb path — the FotMob
// catalog is curated by hand via the admin form. If a future
// source adds its own registry endpoint, plumb a `client` of
// the same shape and call the same EnsureLeague path.
func Discovery(ctx context.Context, repo *Repository, client *sportsdb.Client, logger *slog.Logger) (DiscoveryResult, error) {
	result := DiscoveryResult{Source: "sportsdb"}
	if logger == nil {
		logger = slog.Default()
	}
	if repo == nil || client == nil {
		return result, fmt.Errorf("catalog: discovery requires non-nil repo and client")
	}
	// Some test harnesses wire buildSchedulerDeps without an
	// open DB (e.g. graceful-shutdown test). Discovery is
	// idempotent so re-running on the next boot covers the
	// gap; skip silently instead of panicking the worker.
	if repo.db == nil {
		logger.WarnContext(ctx, "catalog: discovery skipped: repo has no DB handle")
		return result, nil
	}
	leagues, err := client.AllLeagues(ctx)
	if err != nil {
		return result, fmt.Errorf("catalog: discovery fetch leagues: %w", err)
	}
	result.TotalSeen = len(leagues)
	for _, l := range leagues {
		if l.ID == "" {
			result.Skipped++
			continue
		}
		before, err := countLeagues(ctx, repo, l.ID)
		if err != nil {
			return result, fmt.Errorf("catalog: discovery count %s: %w", l.ID, err)
		}
		err = repo.EnsureLeague(ctx, l.ID, scraper.LeagueRef{
			Source:         "sportsdb",
			SourceLeagueId: l.ID,
			Name:           l.Name,
			Sport:          sportsdb.NormalizeSport(l.Sport),
			Country:        l.Country,
		})
		if err != nil {
			result.Skipped++
			logger.WarnContext(ctx, "catalog: discovery skip league",
				slog.String("source_league_id", l.ID),
				slog.String("name", l.Name),
				slog.String("error", err.Error()))
			continue
		}
		after, err := countLeagues(ctx, repo, l.ID)
		if err != nil {
			return result, fmt.Errorf("catalog: discovery recount %s: %w", l.ID, err)
		}
		if after > before {
			result.Upserted++
		} else {
			result.Unchanged++
		}
	}
	logger.InfoContext(ctx, "catalog: discovery done",
		slog.String("source", result.Source),
		slog.Int("upserted", result.Upserted),
		slog.Int("unchanged", result.Unchanged),
		slog.Int("skipped", result.Skipped),
		slog.Int("total_seen", result.TotalSeen),
	)
	return result, nil
}

// countLeagues returns 1 if a row already exists for the given
// (source, source_league_id), else 0. Used by Discovery to tell
// "upserted" from "unchanged" without holding a transaction.
func countLeagues(ctx context.Context, repo *Repository, sourceLeagueID string) (int, error) {
	var row ScraperLeague
	err := repo.db.WithContext(ctx).
		Where("source = ? AND source_league_id = ?", "sportsdb", sourceLeagueID).
		First(&row).Error
	if err == nil {
		return 1, nil
	}
	return 0, nil
}
