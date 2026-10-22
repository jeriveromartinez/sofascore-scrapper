package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
	"golang.org/x/sync/errgroup"
)

const (
	DefaultScrapeConcurrency = 8
	minConcurrency           = 1
	maxConcurrency           = 32
)

// CatalogSource returns the active leagues the scheduler should scrape.
// The DB-backed implementation (*catalog.Repository) lives in
// internal/scraper/catalog; we keep an interface here so this package
// does not import catalog (which would create a cycle, since catalog
// itself imports this package for the LeagueRef type).
//
// ActiveLeaguesBySource is the per-source routing primitive: it
// returns the enabled leagues that belong to the requested source.
// The Service uses it to bucket leagues per registered source
// without grouping by Source post-hoc, which would otherwise
// mis-bucket rows whose override_source names a different source.
type CatalogSource interface {
	ActiveLeagues(ctx context.Context) ([]LeagueRef, error)
	ActiveLeaguesBySource(ctx context.Context, source string) ([]LeagueRef, error)
}

// EnsureLeague is the optional surface a CatalogSource can
// implement to receive auto-created league rows. The bulk-fetch
// dispatch path calls this for any idLeague seen in the
// upstream payload that is not already in the catalog; the
// catalog implementation is responsible for the upsert (and
// for keeping operator-set enabled flags on existing rows).
//
// The bool return distinguishes "newly inserted" from
// "already existed" so the dispatch loop can avoid upserting
// matches for leagues the operator has disabled on a previous
// tick (Fix 4). created=true means a fresh row was inserted;
// created=false means the row already existed (and may be
// disabled).
//
// Catalog backends that want the catalog frozen at boot (e.g.
// the FotMob seed) can simply not implement this interface —
// the dispatch loop type-asserts and skips the auto-create
// step when the assertion fails.
type EnsureLeague interface {
	EnsureLeague(ctx context.Context, sourceLeagueID string, league LeagueRef) (bool, error)
}

// SourceDispatcher routes a league to the source that owns it.
// It is keyed by league.Source (the value of ScraperLeague.Source
// in the catalog: "fotmob", "scores365", ...).
//
// Sources are registered at construction time. The Service uses
// the dispatcher's Lookup to find the right source for each
// league; if no source matches, the league is skipped with a
// warning log (a misconfigured catalog row should not crash the
// scheduler).
type SourceDispatcher struct {
	sources map[string]Source
}

func NewSourceDispatcher(sources ...Source) *SourceDispatcher {
	d := &SourceDispatcher{sources: make(map[string]Source, len(sources))}
	for _, s := range sources {
		d.sources[s.Name()] = s
	}
	return d
}

func (d *SourceDispatcher) Lookup(name string) (Source, bool) {
	s, ok := d.sources[name]
	return s, ok
}

func (d *SourceDispatcher) Names() []string {
	out := make([]string, 0, len(d.sources))
	for n := range d.sources {
		out = append(out, n)
	}
	return out
}

type Service struct {
	repo             *events.Repository
	dispatcher       *SourceDispatcher
	catalog          CatalogSource
	batchSize        int
	concur           int
	onScrapeComplete func(context.Context) error
	logger           *slog.Logger
	now              func() time.Time
}

// NewService builds a Service that dispatches each league to its
// registered source. Pass all sources at construction time — the
// Service iterates the catalog once and groups leagues by source,
// then dispatches each group in parallel.
//
// The first source listed becomes the "primary" used by the
// legacy single-source tests; the Service itself is multi-source.
func NewService(repo *events.Repository, dispatcher *SourceDispatcher, catalog CatalogSource, batchSize int, concurrency int, logger *slog.Logger) (*Service, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("scraper: SourceDispatcher is required")
	}
	if isNilCatalog(catalog) {
		return nil, fmt.Errorf("scraper: catalog repository is required")
	}
	if concurrency == 0 {
		concurrency = DefaultScrapeConcurrency
	}
	if concurrency < minConcurrency || concurrency > maxConcurrency {
		return nil, fmt.Errorf("scraper: concurrency must be between %d and %d, got %d", minConcurrency, maxConcurrency, concurrency)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:       repo,
		dispatcher: dispatcher,
		catalog:    catalog,
		batchSize:  batchSize,
		concur:     concurrency,
		logger:     logger,
		now:        time.Now,
	}, nil
}

func (s *Service) SetOnScrapeComplete(fn func(context.Context) error) {
	s.onScrapeComplete = fn
}

// ScrapeToday iterates each registered source and dispatches its
// leagues in parallel. Per-source routing uses ActiveLeaguesBySource
// so every league reaches the source that published its id.
//
// Sources that implement DayMatcher use the bulk-fetch fast
// path; others iterate their leagues one at a time. Either way,
// the per-league upsert is shared.
func (s *Service) ScrapeToday(ctx context.Context, date time.Time) {
	parentCtx := ctx
	// A plain errgroup (no derived context) keeps a failing source
	// from cancelling its siblings: the first error is still
	// returned by Wait, but the sources running next to it keep
	// their own context alive until they finish.
	var g errgroup.Group
	g.SetLimit(s.concur)
	anyLeagues := false
	for _, srcName := range s.dispatcher.Names() {
		srcName := srcName
		leagues, err := s.catalog.ActiveLeaguesBySource(ctx, srcName)
		if err != nil {
			s.logger.ErrorContext(ctx, "scraper: catalog error",
				slog.String("source", srcName),
				slog.String("error", err.Error()))
			continue
		}
		if len(leagues) == 0 {
			s.logger.WarnContext(ctx, "scraper: no active leagues for source",
				slog.String("source", srcName))
			continue
		}
		anyLeagues = true
		src, ok := s.dispatcher.Lookup(srcName)
		if !ok {
			// Defensive: Names() returns keys already in the
			// dispatcher's map, so Lookup must succeed. If a
			// race ever lets it fail we just skip that source.
			continue
		}
		g.Go(func() error {
			return s.dispatchSource(ctx, src, leagues, date, parentCtx)
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape today errors", slog.String("error", err.Error()))
	}
	if !anyLeagues {
		s.logger.WarnContext(ctx, "scraper: no active leagues in catalog")
	}
	if s.onScrapeComplete != nil {
		if hookErr := s.onScrapeComplete(parentCtx); hookErr != nil {
			// The hook typically publishes "scrape completed"
			// notifications or writes a Redis marker. A failure
			// here means downstream consumers never learn the
			// scrape ran — surface it loudly rather than swallowing.
			s.logger.ErrorContext(parentCtx, "scraper: onScrapeComplete hook failed",
				slog.String("error", hookErr.Error()))
		}
	}
}

// ScrapeNext7Days is the lookahead variant. Each day is fetched
// independently so a single per-day failure does not poison the
// rest of the lookahead window. The per-source loop mirrors
// ScrapeToday so override_source routing applies to the
// lookahead window as well.
func (s *Service) ScrapeNext7Days(ctx context.Context) {
	now := s.now()
	for i := 1; i <= 7; i++ {
		// Calendar-day arithmetic: adding a fixed 24h skips a day
		// (or repeats one) across a DST transition.
		date := addDays(now, i)
		parentCtx := ctx
		var g errgroup.Group
		g.SetLimit(s.concur)
		for _, srcName := range s.dispatcher.Names() {
			srcName := srcName
			leagues, err := s.catalog.ActiveLeaguesBySource(ctx, srcName)
			if err != nil {
				s.logger.ErrorContext(ctx, "scraper: catalog error",
					slog.String("source", srcName),
					slog.String("error", err.Error()))
				continue
			}
			if len(leagues) == 0 {
				continue
			}
			src, ok := s.dispatcher.Lookup(srcName)
			if !ok {
				continue
			}
			g.Go(func() error {
				return s.dispatchSource(ctx, src, leagues, date, parentCtx)
			})
		}
		if err := g.Wait(); err != nil {
			s.logger.ErrorContext(ctx, "scrape next 7 days errors",
				slog.String("date", date.Format("2006-01-02")),
				slog.String("error", err.Error()))
		}
		if s.onScrapeComplete != nil {
			if hookErr := s.onScrapeComplete(parentCtx); hookErr != nil {
				// See ScrapeToday: log rather than swallow so a
				// failing Redis publish / notifier is visible.
				s.logger.ErrorContext(parentCtx, "scraper: onScrapeComplete hook failed",
					slog.String("error", hookErr.Error()))
			}
		}
	}
}

// addDays advances a timestamp by n calendar days, keeping the
// wall-clock time. A fixed 24h offset is not the same thing: on the
// days a DST transition falls in, it lands on the wrong calendar day.
func addDays(t time.Time, n int) time.Time {
	return t.AddDate(0, 0, n)
}

// dispatchSource drives one source's leagues for a single day.
// When the source implements DayMatcher (the FotMob and
// scores365 bulk-fetch fast path), one HTTP call covers all
// leagues in srcLeagues. Otherwise the source is iterated
// league-by-league (the per-league path, which makes one HTTP
// call per league under the hood).
func (s *Service) dispatchSource(
	ctx context.Context,
	src Source,
	srcLeagues []LeagueRef,
	date time.Time,
	parentCtx context.Context,
) error {
	if len(srcLeagues) == 0 {
		return nil
	}
	if dm, ok := src.(DayMatcher); ok {
		return s.dispatchDayMatch(ctx, dm, src, srcLeagues, date, parentCtx)
	}
	return s.dispatchPerLeague(ctx, src, srcLeagues, date, parentCtx)
}

// dispatchDayMatch handles the bulk-fetch path: one HTTP call,
// N league buckets, parallel upserts. For sources whose catalog
// is intentionally narrow (FotMob, football-only) the bucket
// loop iterates srcLeagues and drops unknown-league matches.
//
// For sources whose catalog is the full universe of upstream
// leagues (currently scores365), the catalog can grow on the
// fly: any league ID seen in the day payload that is not yet
// in the catalog is upserted via ensureLeague (enabled=true),
// so the admin sees it on the next dashboard load and can
// choose to disable it. The auto-create path is guarded by
// both an interface assertion and a per-source gate
// (`src.Name()` must be a source whose upstream publishes
// events for every league — currently just scores365) so the
// FotMob fast-path keeps its stricter behaviour.
func (s *Service) dispatchDayMatch(
	ctx context.Context,
	dm DayMatcher,
	src Source,
	srcLeagues []LeagueRef,
	date time.Time,
	parentCtx context.Context,
) error {
	start := time.Now()
	matches, err := dm.DayMatches(ctx, date)
	if err != nil {
		s.logger.ErrorContext(ctx, "scrape day: dayMatches",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		for _, league := range srcLeagues {
			recordLeagueScrape(league, start, err)
		}
		return err
	}
	byLeague := make(map[string][]Match, len(srcLeagues))
	for _, m := range matches {
		byLeague[m.League.SourceLeagueId] = append(byLeague[m.League.SourceLeagueId], m)
	}
	known := make(map[string]struct{}, len(srcLeagues))
	for _, l := range srcLeagues {
		known[l.SourceLeagueId] = struct{}{}
	}
	for id := range byLeague {
		if _, ok := known[id]; ok {
			continue
		}
		s.logger.WarnContext(ctx, "scrape day: unknown league from source",
			slog.String("source", src.Name()),
			slog.String("source_league_id", id))
	}

	// The leagues are upserted in parallel; a league that fails
	// must not cancel the ones running next to it, so the group
	// runs without a derived context.
	var g errgroup.Group
	g.SetLimit(s.concur)

	// Step 1: enabled leagues from the catalog. Their canonical
	// sport/country override the upstream payload's verbose
	// strings on the way to the upsert.
	for _, league := range srcLeagues {
		league := league
		g.Go(func() error {
			err := s.upsertLeagueMatches(ctx, league, date, byLeague[league.SourceLeagueId])
			recordLeagueScrape(league, start, err)
			return err
		})
	}

	// Step 2: league IDs present in the day payload that are
	// not yet in the catalog. Only run when the catalog supports
	// dynamic auto-create (the interface assertion below) AND
	// the source is one whose upstream publishes events for
	// every league (currently scores365). The FotMob path is
	// gated off so its curated narrow catalog stays narrow.
	//
	// Auto-create only for sources whose upstream publishes events
	// for every league (currently just scores365). The FotMob path
	// is curated and must not silently grow the catalog.
	if el, ok := s.catalog.(EnsureLeague); ok && src.Name() == "scores365" {
		for _, ms := range byLeague {
			if len(ms) == 0 {
				continue
			}
			first := ms[0]
			// Fix 3 (P1): football is FotMob's primary source. The
			// discovery job deliberately omits football competitions
			// for that reason; auto-enrol must mirror the same
			// boundary so a stray football league in the daily
			// feed does not sneak into the catalog.
			if first.League.Sport == "football" {
				continue
			}
			id := first.League.SourceLeagueId
			// Skip leagues already handled by step 1.
			if _, known := byLeagueToRef(srcLeagues, id); known {
				continue
			}
			league := first.League // canonical sport already from NormalizeSport
			g.Go(func() error {
				created, err := el.EnsureLeague(ctx, id, league)
				if err != nil {
					s.logger.WarnContext(ctx, "scrape: ensure league failed",
						slog.String("source", src.Name()),
						slog.String("source_league_id", id),
						slog.String("error", err.Error()))
					recordLeagueScrape(league, start, err)
					return nil
				}
				// Fix 4 (P1): only push matches when the row was
				// newly inserted. A pre-existing row means the
				// operator may have disabled it on a previous tick;
				// upserting matches for it would silently re-enable
				// the league, defeating the opt-out.
				if !created {
					return nil
				}
				err = s.upsertLeagueMatches(ctx, league, date, byLeague[id])
				recordLeagueScrape(league, start, err)
				return err
			})
		}
	}

	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape day: league upserts",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		return err
	}
	return nil
}

// byLeagueToRef is a tiny helper: returns (league, true) when
// id is in srcLeagues. Avoids a map allocation on the hot path.
func byLeagueToRef(srcLeagues []LeagueRef, id string) (LeagueRef, bool) {
	for _, l := range srcLeagues {
		if l.SourceLeagueId == id {
			return l, true
		}
	}
	return LeagueRef{}, false
}

// dispatchPerLeague handles sources that don't expose a bulk
// per-day fetch. Each league is fetched with its own HTTP call
// under the source's per-source rate limiter.
func (s *Service) dispatchPerLeague(
	ctx context.Context,
	src Source,
	srcLeagues []LeagueRef,
	date time.Time,
	parentCtx context.Context,
) error {
	// As in dispatchDayMatch: the leagues run in parallel and a
	// failure must not cancel the siblings.
	var g errgroup.Group
	g.SetLimit(s.concur)
	for _, league := range srcLeagues {
		league := league
		g.Go(func() error {
			start := time.Now()
			matches, err := src.ScheduledEvents(ctx, league, date)
			if err != nil {
				recordLeagueScrape(league, start, err)
				s.logger.ErrorContext(ctx, "scrape per-league",
					slog.String("source", src.Name()),
					slog.String("source_league_id", league.SourceLeagueId),
					slog.String("date", date.Format("2006-01-02")),
					slog.String("error", err.Error()))
				return err
			}
			err = s.upsertLeagueMatches(ctx, league, date, matches)
			recordLeagueScrape(league, start, err)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape per-league group errors",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		return err
	}
	return nil
}

// upsertLeagueMatches is the shared tail of both dispatch paths:
// build a ScrapeBatch from the supplied matches and upsert it.
// sport is taken from the league ref so the row carries the right
// category.
func (s *Service) upsertLeagueMatches(ctx context.Context, league LeagueRef, date time.Time, matches []Match) error {
	if len(matches) == 0 {
		s.logger.DebugContext(ctx, "scraper: no matches for league",
			slog.String("league_id", league.SourceLeagueId))
		return nil
	}
	sport := leagueSport(league)
	batch, err := ToScrapeBatch(matches, sport)
	if err != nil {
		return fmt.Errorf("scraper: build batch for %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
	if err := s.repo.UpsertScrapeBatch(ctx, batch, s.batchSize); err != nil {
		return fmt.Errorf("scraper: upsert %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
	observability.ScrapeEvents.WithLabelValues(sport).Add(float64(len(batch.Events)))
	return nil
}

func leagueSport(league LeagueRef) string {
	if league.Sport == "" {
		return "football"
	}
	return league.Sport
}

// recordLeagueScrape publishes the outcome of one league's scrape for
// one day. On the bulk-fetch path start is taken before the shared day
// fetch, so the duration covers fetch plus upsert on both paths.
func recordLeagueScrape(league LeagueRef, start time.Time, err error) {
	sport := leagueSport(league)
	status := "ok"
	if err != nil {
		status = "error"
	}
	observability.ScrapeRequests.WithLabelValues(sport, status).Inc()
	observability.ScrapeDuration.WithLabelValues(sport).Observe(time.Since(start).Seconds())
}

// isNilCatalog reports whether the catalog dependency is missing.
// A typed nil pointer (e.g. (*catalog.Repository)(nil)) is not equal
// to a nil interface, so the pointer case is checked explicitly.
func isNilCatalog(c CatalogSource) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}
