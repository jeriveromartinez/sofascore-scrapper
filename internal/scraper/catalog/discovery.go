// Package catalog houses the bulk-discovery glue that keeps the
// admin scraper catalog in sync with the upstream (365scores)
// league registry.
//
// Background: the scraper dispatches every FotMob league per
// call and every scores365 league via the bulk-fetch day path
// (DayMatches). Both paths require the catalog to know the
// league up front — the bulk path drops any match whose
// idLeague is not in `scraper_leagues`.
//
// This package fixes that with a single-shot discovery job
// that pulls the upstream 365scores sitemaps, normalises the
// sport names, and upserts every row into `scraper_leagues`
// with enabled=true. Operators can flip a row off later via
// the existing admin UI.
package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scores365"
)

// DiscoveryResult is the per-run summary logged so operators
// can see what was added on each invocation.
type DiscoveryResult struct {
	Upserted  int // newly inserted rows
	Unchanged int // rows that already existed (idempotent path)
	Skipped   int // rows the upstream published but with no id
	TotalSeen int // total rows the upstream returned
	Source    string
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

// sportSlugsForDiscovery is the list of 365scores sport slugs we probe.
// Order matches the boot-time fan-out. Football (slug=football) is omitted
// because FotMob remains the primary source for football.
var sportSlugsForDiscovery = []string{
	"basketball",
	"tennis",
	"hockey",
	"american-football",
	"baseball",
	"volleyball",
}

// Discovery sweeps the upstream 365scores sitemaps and upserts
// every row into `scraper_leagues`. Idempotent: re-running on
// the same sitemap set is a no-op (the unique key on
// (source, source_league_id) blocks duplicates and EnsureLeague
// skips already-present rows).
func Discovery(ctx context.Context, repo *Repository, client *scores365.Client, logger *slog.Logger) (DiscoveryResult, error) {
	result := DiscoveryResult{Source: "scores365"}
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
	for _, sport := range sportSlugsForDiscovery {
		sportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		leagues, err := client.FetchSitemap(sportCtx, "en", sport)
		cancel()
		if err != nil {
			logger.WarnContext(ctx, "catalog: discovery failed for sport",
				slog.String("sport", sport),
				slog.String("error", err.Error()))
			continue
		}
		for _, l := range leagues {
			result.TotalSeen++
			before, err := repo.ExistsBySourceLeagueID(ctx, l.Source, l.SourceLeagueId)
			if err != nil {
				return result, fmt.Errorf("catalog: discovery exists check %s/%s: %w", l.Source, l.SourceLeagueId, err)
			}
			err = repo.EnsureLeague(ctx, l.SourceLeagueId, toLeagueRef(l))
			if err != nil {
				result.Skipped++
				logger.WarnContext(ctx, "catalog: discovery skip league",
					slog.String("source_league_id", l.SourceLeagueId),
					slog.String("name", l.Name),
					slog.String("error", err.Error()))
				continue
			}
			after, err := repo.ExistsBySourceLeagueID(ctx, l.Source, l.SourceLeagueId)
			if err != nil {
				return result, fmt.Errorf("catalog: discovery recheck: %w", err)
			}
			if after > before {
				result.Upserted++
			} else {
				result.Unchanged++
			}
		}
	}
	logger.InfoContext(ctx, "catalog: discovery done", slog.Any("discovery", result))
	return result, nil
}

// toLeagueRef is the bridge between the upstream sitemap rows and
// the catalog schema. The upstream comp ID (e.g. "47") is the
// 365scores raw competition ID; we prefix it with LeagueIDPrefix
// so it lands in the same ID space as the tournaments rows seeded
// by eventToMatch (e.g. "6000000047"). Without this prefix FotMob
// league ID 47 (Premier League) and 365scores comp ID 47 (NBA
// sitemap fixture) would collide on the tournaments primary key.
//
// The catalog league row is keyed by (source, source_league_id)
// so the prefix only affects the tournaments join, not the
// scraper_leagues row itself.
func toLeagueRef(l scores365.League) scraper.LeagueRef {
	id := l.SourceLeagueId
	if raw, err := strconv.ParseInt(id, 10, 64); err == nil {
		id = strconv.FormatInt(scores365.LeagueIDPrefix+raw, 10)
	}
	return scraper.LeagueRef{
		Source:         l.Source,
		SourceLeagueId: id,
		Name:           l.Name,
		Sport:          l.Sport,
		Country:        l.Country,
	}
}

