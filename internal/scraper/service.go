package scraper

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"golang.org/x/sync/errgroup"
)

const (
	DefaultScrapeConcurrency = 8
	minConcurrency           = 1
	maxConcurrency           = 32
)

type Service struct {
	repo      *events.Repository
	client    SofaScoreClient
	batchSize int
	concur    int
}

func NewService(repo *events.Repository, client SofaScoreClient, batchSize int, concurrency int) (*Service, error) {
	if concurrency == 0 {
		concurrency = DefaultScrapeConcurrency
	}
	if concurrency < minConcurrency || concurrency > maxConcurrency {
		return nil, fmt.Errorf("scraper: concurrency must be between %d and %d, got %d", minConcurrency, maxConcurrency, concurrency)
	}
	return &Service{
		repo:      repo,
		client:    client,
		batchSize: batchSize,
		concur:    concurrency,
	}, nil
}

func (s *Service) Scrape(ctx context.Context, sport string, date time.Time) error {
	apiEvents, err := s.client.ScheduledEvents(ctx, sport, date)
	if err != nil {
		return fmt.Errorf("scraper: %s on %s: %w", sport, date.Format("2006-01-02"), err)
	}
	batch := ToScrapeBatch(apiEvents, sport)
	if err := s.repo.UpsertScrapeBatch(ctx, batch, s.batchSize); err != nil {
		return fmt.Errorf("scraper: upsert %s on %s: %w", sport, date.Format("2006-01-02"), err)
	}
	log.Printf("scraper: scraped %d events for %s on %s", len(apiEvents), sport, date.Format("2006-01-02"))
	return nil
}

func (s *Service) ScrapeCountry(ctx context.Context, countryCode string) error {
	events, err := s.client.TrendingEvents(ctx, countryCode)
	if err != nil {
		return fmt.Errorf("scraper: country %s: %w", countryCode, err)
	}
	batch := ToScrapeBatch(events, countryCode)
	if err := s.repo.UpsertScrapeBatch(ctx, batch, s.batchSize); err != nil {
		return fmt.Errorf("scraper: upsert country %s: %w", countryCode, err)
	}
	log.Printf("scraper: scraped %d events for country %s", len(events), countryCode)
	return nil
}

func (s *Service) ScrapeToday(ctx context.Context, date time.Time) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)

	for _, sport := range GET_SPORTS() {
		sport := sport
		g.Go(func() error {
			return s.Scrape(ctx, sport, date)
		})
	}
	for _, country := range GET_COUNTRIES() {
		country := country
		g.Go(func() error {
			return s.ScrapeCountry(ctx, country)
		})
	}

	if err := g.Wait(); err != nil {
		log.Printf("scraper: scrape today errors: %v", err)
	}
}

func (s *Service) ScrapeNext7Days(ctx context.Context) {
	now := time.Now()
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concur)

	for _, sport := range GET_SPORTS() {
		sport := sport
		for i := 1; i <= 7; i++ {
			i := i
			g.Go(func() error {
				return s.Scrape(ctx, sport, now.Add(time.Duration(i)*24*time.Hour))
			})
		}
	}

	if err := g.Wait(); err != nil {
		log.Printf("scraper: scrape next 7 days errors: %v", err)
	}
}
