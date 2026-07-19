package scraper

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
)

type Service struct {
	repo      *events.Repository
	batchSize int
}

func NewService(repo *events.Repository, batchSize int) *Service {
	return &Service{repo: repo, batchSize: batchSize}
}

func (s *Service) Scrape(sport string, date time.Time) {
	body := LoadDataBySport(sport, date)
	var list EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		log.Printf("scraper: error parsing JSON for %s on %s: %v", sport, date.Format("2006-01-02"), err)
		return
	}
	batch := ToScrapeBatch(list.Events, sport)
	if err := s.repo.UpsertScrapeBatch(context.Background(), batch, s.batchSize); err != nil {
		log.Printf("scraper: error upserting events for %s on %s: %v", sport, date.Format("2006-01-02"), err)
		return
	}
	log.Printf("scraper: scraped %d events for %s on %s", len(list.Events), sport, date.Format("2006-01-02"))
}

func (s *Service) ScrapeCountry(countryCode string) {
	body := LoadDataByTrendingCountry(countryCode)
	var list EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		log.Printf("scraper: error parsing JSON for country %s: %v", countryCode, err)
		return
	}
	batch := ToScrapeBatch(list.Events, countryCode)
	if err := s.repo.UpsertScrapeBatch(context.Background(), batch, s.batchSize); err != nil {
		log.Printf("scraper: error upserting events for country %s: %v", countryCode, err)
		return
	}
	log.Printf("scraper: scraped %d events for country %s", len(list.Events), countryCode)
}
