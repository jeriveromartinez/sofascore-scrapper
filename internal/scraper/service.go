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

type Service struct {
	repo             *events.Repository
	source           Source
	catalog          CatalogSource
	batchSize        int
	concur           int
	onScrapeComplete func(context.Context) error
	logger           *slog.Logger
}

func NewService(repo *events.Repository, source Source, catalog CatalogSource, batchSize int, concurrency int, logger *slog.Logger) (*Service, error) {
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
		repo:      repo,
		source:    source,
		catalog:   catalog,
		batchSize: batchSize,
		concur:    concurrency,
		logger:    logger,
	}, nil
}

func (s *Service) SetOnScrapeComplete(fn func(context.Context) error) {
	s.onScrapeComplete = fn
}

func (s *Service) scrapeLeague(ctx context.Context, league LeagueRef, date time.Time) error {
	matches, err := s.source.ScheduledEvents(ctx, league, date)
	if err != nil {
		return fmt.Errorf("scraper: %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
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

	// Capture the parent ctx BEFORE errgroup.WithContext so the
	// completion hook sees a live context. errgroup.WithContext
	// derives a ctx that gets canceled when g.Wait() returns, which
	// means the hook used to always receive a canceled context —
	// Redis (epoch increment) and similar IO from the hook would
	// fail immediately. Fix B5 (PR #122).
	parentCtx := ctx
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)
	for _, league := range leagues {
		league := league
		g.Go(func() error {
			return s.scrapeLeague(ctx, league, date)
		})
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape today errors", slog.String("error", err.Error()))
	}
	if s.onScrapeComplete != nil {
		_ = s.onScrapeComplete(parentCtx)
	}
}

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
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)
	for _, league := range leagues {
		league := league
		for i := 1; i <= 7; i++ {
			i := i
			g.Go(func() error {
				return s.scrapeLeague(ctx, league, now.Add(time.Duration(i)*24*time.Hour))
			})
		}
	}
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "scrape next 7 days errors", slog.String("error", err.Error()))
	}
}
