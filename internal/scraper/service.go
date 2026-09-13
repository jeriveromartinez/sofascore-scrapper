package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
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
type CatalogSource interface {
	ActiveLeagues(ctx context.Context) ([]LeagueRef, error)
}

// SourceDispatcher routes a league to the source that owns it.
// It is keyed by league.Source (the value of ScraperLeague.Source
// in the catalog: "fotmob", "sportsdb", ...).
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
	}, nil
}

func (s *Service) SetOnScrapeComplete(fn func(context.Context) error) {
	s.onScrapeComplete = fn
}

// ScrapeToday groups the active leagues by source and dispatches
// each group in parallel. Sources that implement DayMatcher use
// the bulk-fetch fast path; others iterate their leagues one at a
// time. Either way, the per-league upsert is shared.
func (s *Service) ScrapeToday(ctx context.Context, date time.Time) {
	leagues, err := s.catalog.ActiveLeagues(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "scraper: catalog error", slog.String("error", err.Error()))
		return
	}
	if len(leagues) == 0 {
		s.logger.WarnContext(ctx, "scraper: no active leagues in catalog")
		return
	}
	parentCtx := ctx
	s.dispatchAll(ctx, date, leagues, parentCtx)
}

// ScrapeNext7Days is the lookahead variant. Each day is fetched
// independently so a single per-day failure does not poison the
// rest of the lookahead window.
func (s *Service) ScrapeNext7Days(ctx context.Context) {
	leagues, err := s.catalog.ActiveLeagues(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "scraper: catalog error", slog.String("error", err.Error()))
		return
	}
	if len(leagues) == 0 {
		return
	}

	now := time.Now()
	for i := 1; i <= 7; i++ {
		date := now.Add(time.Duration(i) * 24 * time.Hour)
		s.dispatchAll(ctx, date, leagues, ctx)
	}
}

// dispatchAll groups leagues by source.Source and runs each
// group's dispatch in parallel. The completion hook fires once
// per call after every group has finished.
func (s *Service) dispatchAll(ctx context.Context, date time.Time, leagues []LeagueRef, parentCtx context.Context) {
	grouped := s.groupBySource(leagues)
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)
	for srcName, srcLeagues := range grouped {
		srcName := srcName
		srcLeagues := srcLeagues
		g.Go(func() error {
			src, ok := s.dispatcher.Lookup(srcName)
			if !ok {
				s.logger.WarnContext(ctx, "scraper: no source registered for catalog row",
					slog.String("source", srcName),
					slog.Int("league_count", len(srcLeagues)))
				return nil
			}
			return s.dispatchSource(ctx, src, srcLeagues, date, parentCtx)
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape today errors", slog.String("error", err.Error()))
	}
	if s.onScrapeComplete != nil {
		_ = s.onScrapeComplete(parentCtx)
	}
}

// groupBySource buckets leagues by their Source field while
// filtering out any league whose Source is not registered. We log
// a warning per unknown source so misconfigured catalog rows are
// visible without crashing the cron.
func (s *Service) groupBySource(leagues []LeagueRef) map[string][]LeagueRef {
	out := make(map[string][]LeagueRef)
	for _, l := range leagues {
		if _, ok := s.dispatcher.Lookup(l.Source); !ok {
			s.logger.Warn("scraper: league references unknown source, skipping",
				slog.String("source", l.Source),
				slog.String("source_league_id", l.SourceLeagueId),
				slog.String("league_name", l.Name))
			continue
		}
		out[l.Source] = append(out[l.Source], l)
	}
	return out
}

// dispatchSource drives one source's leagues for a single day.
// When the source implements DayMatcher (the FotMob fast path),
// one HTTP call covers all leagues in srcLeagues. Otherwise the
// source is iterated league-by-league (the TheSportsDB path,
// which makes one HTTP call per league under the hood).
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
// N league buckets, parallel upserts. Matches whose
// League.SourceLeagueId is not in srcLeagues are silently dropped
// (e.g. a competitor league that happens to share the same
// upstream day payload).
func (s *Service) dispatchDayMatch(
	ctx context.Context,
	dm DayMatcher,
	src Source,
	srcLeagues []LeagueRef,
	date time.Time,
	parentCtx context.Context,
) error {
	matches, err := dm.DayMatches(ctx, date)
	if err != nil {
		s.logger.ErrorContext(ctx, "scrape day: dayMatches",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		return err
	}
	byLeague := make(map[string][]Match, len(srcLeagues))
	for _, m := range matches {
		byLeague[m.League.SourceLeagueId] = append(byLeague[m.League.SourceLeagueId], m)
	}
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)
	for _, league := range srcLeagues {
		league := league
		g.Go(func() error {
			return s.upsertLeagueMatches(ctx, league, date, byLeague[league.SourceLeagueId])
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape day: league upserts",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		return err
	}
	if s.onScrapeComplete != nil {
		_ = s.onScrapeComplete(parentCtx)
	}
	return nil
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
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)
	for _, league := range srcLeagues {
		league := league
		g.Go(func() error {
			matches, err := src.ScheduledEvents(ctx, league, date)
			if err != nil {
				s.logger.ErrorContext(ctx, "scrape per-league",
					slog.String("source", src.Name()),
					slog.String("source_league_id", league.SourceLeagueId),
					slog.String("date", date.Format("2006-01-02")),
					slog.String("error", err.Error()))
				return err
			}
			return s.upsertLeagueMatches(ctx, league, date, matches)
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape per-league group errors",
			slog.String("source", src.Name()),
			slog.String("date", date.Format("2006-01-02")),
			slog.String("error", err.Error()))
		return err
	}
	if s.onScrapeComplete != nil {
		_ = s.onScrapeComplete(parentCtx)
	}
	return nil
}

// upsertLeagueMatches is the shared tail of both dispatch paths:
// build a ScrapeBatch from the supplied matches and upsert it.
// sport is taken from the league ref so the row carries the right
// category.
func (s *Service) upsertLeagueMatches(ctx context.Context, league LeagueRef, date time.Time, matches []Match) error {
	if len(matches) == 0 {
		return nil
	}
	sport := league.Sport
	if sport == "" {
		sport = "football"
	}
	batch := ToScrapeBatch(matches, sport)
	if err := s.repo.UpsertScrapeBatch(ctx, batch, s.batchSize); err != nil {
		return fmt.Errorf("scraper: upsert %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
	return nil
}
