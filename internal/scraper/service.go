package scraper

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
)

type Service struct {
	repo *events.Repository
}

func NewService(repo *events.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Scrape(sport string, date time.Time) {
	body := LoadDataBySport(sport, date)
	var list EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		log.Printf("scraper: error parsing JSON for %s on %s: %v", sport, date.Format("2006-01-02"), err)
		return
	}
	evs := make([]events.Event, 0, len(list.Events))
	for _, apiEvent := range list.Events {
		evs = append(evs, ToEvent(*apiEvent, sport))
	}
	if err := s.repo.Upsert(context.Background(), evs, sport); err != nil {
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
	evs := make([]events.Event, 0, len(list.Events))
	for _, apiEvent := range list.Events {
		evs = append(evs, ToEvent(*apiEvent, countryCode))
	}
	if err := s.repo.Upsert(context.Background(), evs, countryCode); err != nil {
		log.Printf("scraper: error upserting events for country %s: %v", countryCode, err)
		return
	}
	log.Printf("scraper: scraped %d events for country %s", len(list.Events), countryCode)
}
