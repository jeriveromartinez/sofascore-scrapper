// Package catalog houses the bulk-discovery glue that keeps the
// admin scraper catalog in sync with the upstream (365scores)
// league registry.
//
// Background: the scraper dispatches every FotMob league per
// call and every scores365 league via the bulk-fetch day path
// (DayMatches). Both paths require the catalog to know the
// league up front — the bulk path drops any match whose
// league is not in `scraper_leagues`.
//
// This package fixes that with a single-shot discovery job that
// pulls the upstream competition catalog from the 365scores web API
// (/web/competitions/), normalises the sport names, and upserts every
// row into `scraper_leagues` with enabled=true. Operators can flip a
// row off later via the existing admin UI.
//
// The discovery sweep reads JSON from the same API the site itself
// calls; nothing here parses markup.
package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
	scraper365 "github.com/jeriveromartinez/scores365-scrapper/internal/scraper/scores365"
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

// DefaultSportSlugs is the list of sports the discovery sweep
// enrols. Football is omitted because FotMob remains the primary
// source for football; adding it here would enrol ~1.5k football
// competitions that the dispatcher would then have to route away
// from FotMob.
var DefaultSportSlugs = []string{
	"basketball",
	"tennis",
	"ice-hockey",
	"handball",
	"american-football",
	"baseball",
	"volleyball",
	"rugby",
}

// discoveryTimeout bounds the single competitions request. The
// catalog endpoint returns ~2.7k rows (~1.3MB uncompressed, well
// under 200KB gzipped), so one request replaces the previous
// per-sport fan-out.
const discoveryTimeout = 45 * time.Second

// Discovery pulls the upstream 365scores competition catalog and
// upserts every row into `scraper_leagues`. Idempotent: re-running
// against the same catalog is a no-op (the unique key on
// (source, source_league_id) blocks duplicates and EnsureLeague
// skips already-present rows).
func Discovery(ctx context.Context, repo *Repository, client *scores365.Client, logger *slog.Logger) (DiscoveryResult, error) {
	return DiscoverySports(ctx, repo, client, DefaultSportSlugs, logger)
}

// DiscoverySports is Discovery with an explicit sport-slug list, so
// a deployment can widen or narrow the sweep from configuration.
func DiscoverySports(
	ctx context.Context,
	repo *Repository,
	client *scores365.Client,
	sportSlugs []string,
	logger *slog.Logger,
) (DiscoveryResult, error) {
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
	if len(sportSlugs) == 0 {
		sportSlugs = DefaultSportSlugs
	}

	sportIDs, wanted := resolveSports(sportSlugs, logger)
	if len(sportIDs) == 0 {
		logger.WarnContext(ctx, "catalog: discovery skipped: no known sports configured")
		return result, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	feed, err := client.FetchCompetitions(fetchCtx, sportIDs)
	cancel()
	if err != nil {
		logger.WarnContext(ctx, "catalog: discovery fetch failed",
			slog.String("error", err.Error()))
		return result, fmt.Errorf("catalog: discovery fetch competitions: %w", err)
	}

	countries := make(map[int]string, len(feed.Countries))
	for _, c := range feed.Countries {
		countries[c.ID] = c.Name
	}

	for _, comp := range feed.Competitions {
		// The API honours ?sports= but the response also carries the
		// dictionaries it needed to build the rows, so filter again
		// defensively: enrolling a football competition here would
		// take fixtures away from FotMob.
		if !wanted[comp.SportID] {
			continue
		}
		result.TotalSeen++
		if comp.ID == 0 {
			result.Skipped++
			continue
		}
		l := toLeague(comp, countries)
		// The catalog row is keyed by the *namespaced* league id
		// because that is what the scrape loop matches against:
		// dispatchDayMatches buckets the day feed by
		// Match.League.SourceLeagueId, which gameToMatch builds as
		// LeagueIDPrefix+compId. Storing the raw comp id here would
		// leave every discovered row unmatched and let the
		// auto-enrol path insert a second, prefixed row beside it.
		ref := toLeagueRef(l)

		before, err := repo.ExistsBySourceLeagueID(ctx, ref.Source, ref.SourceLeagueId)
		if err != nil {
			return result, fmt.Errorf("catalog: discovery exists check %s/%s: %w", ref.Source, ref.SourceLeagueId, err)
		}
		created, err := repo.EnsureLeague(ctx, ref.SourceLeagueId, ref)
		if err != nil {
			result.Skipped++
			logger.WarnContext(ctx, "catalog: discovery skip league",
				slog.String("source_league_id", ref.SourceLeagueId),
				slog.String("name", ref.Name),
				slog.String("error", err.Error()))
			continue
		}
		after, err := repo.ExistsBySourceLeagueID(ctx, ref.Source, ref.SourceLeagueId)
		if err != nil {
			return result, fmt.Errorf("catalog: discovery recheck: %w", err)
		}
		if created || after > before {
			result.Upserted++
		} else {
			result.Unchanged++
		}
	}

	logger.InfoContext(ctx, "catalog: discovery done", slog.Any("discovery", result))
	return result, nil
}

// resolveSports turns the configured slug list into the numeric
// sportIds the competitions endpoint filters on, plus the set used
// to re-filter the response. Unknown slugs are logged and dropped
// rather than failing the whole sweep.
func resolveSports(slugs []string, logger *slog.Logger) ([]int, map[int]bool) {
	ids := make([]int, 0, len(slugs))
	wanted := make(map[int]bool, len(slugs))
	for _, slug := range slugs {
		id := scraper365.SportIDForSlug(slug)
		if id == 0 {
			logger.Warn("catalog: discovery unknown sport slug", slog.String("sport", slug))
			continue
		}
		if wanted[id] {
			continue
		}
		wanted[id] = true
		ids = append(ids, id)
	}
	return ids, wanted
}

// toLeague normalises an upstream competition row into the shape the
// catalog consumes. The country name comes from the dictionary the
// same response carries; international competitions (countryId
// pointing at a synthetic "World"/"Europe" row) resolve the same way.
func toLeague(comp scores365.Competition, countries map[int]string) scores365.League {
	name := comp.Name
	if name == "" {
		name = comp.ShortName
	}
	return scores365.League{
		Source:         "scores365",
		SourceLeagueId: strconv.Itoa(comp.ID),
		Name:           name,
		Sport:          scraper365.SportSlug(comp.SportID),
		Country:        countries[comp.CountryID],
		NameForURL:     comp.NameForURL,
	}
}

// toLeagueRef is the bridge between the upstream competition rows and
// the catalog schema. The upstream comp ID (e.g. "47") is the
// 365scores raw competition ID; we prefix it with LeagueIDPrefix
// so it lands in the same ID space as the tournaments rows seeded
// by gameToMatch (e.g. "6000000047"). Without this prefix FotMob
// league ID 47 (Premier League) and 365scores comp ID 47 would
// collide on the tournaments primary key.
//
// The prefixed id is also what the scraper_leagues row is keyed
// by, so a discovered row matches the day feed's bucket key in
// dispatchDayMatches without any further translation.
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
