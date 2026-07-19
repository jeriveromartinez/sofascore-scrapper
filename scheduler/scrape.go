package scheduler

import (
	"log"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func newScraperService() *scraper.Service {
	db, err := database.GetDB()
	if err != nil {
		log.Printf("scheduler: failed to get DB for scraper: %v", err)
		return nil
	}
	return scraper.NewService(events.NewRepository(db))
}

func scrape(sport string, date time.Time) {
	svc := newScraperService()
	if svc == nil {
		return
	}
	svc.Scrape(sport, date)
}

func scrapeCountry(countryCode string) {
	svc := newScraperService()
	if svc == nil {
		return
	}
	svc.ScrapeCountry(countryCode)
}

func scrapeToday(date time.Time) {
	for _, sport := range scraper.GET_SPORTS() {
		scrape(sport, date)
	}
	for _, country := range scraper.GET_COUNTRIES() {
		scrapeCountry(country)
	}
}

func scrapeNext7Days() {
	now := time.Now()
	for _, sport := range scraper.GET_SPORTS() {
		for i := 1; i <= 7; i++ {
			scrape(sport, now.Add(time.Duration(i)*24*time.Hour))
		}
	}
}

func startScrape() {
	go scrapeToday(time.Now())
	go scrapeNext7Days()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			scrape(scraper.FOOTBALL, time.Now())
		}
	}()

	go func() {
		for {
			now := time.Now().UTC()
			h, m, s := now.Clock()
			var next time.Time
			if h < 6 || (h == 6 && m == 0 && s == 0) {
				next = time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
			} else if h < 18 || (h == 18 && m == 0 && s == 0) {
				next = time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, time.UTC)
			} else {
				next = time.Date(now.Year(), now.Month(), now.Day()+1, 6, 0, 0, 0, time.UTC)
			}
			if !next.After(now) {
				next = next.Add(time.Second)
			}
			time.Sleep(time.Until(next))
			scrapeNext7Days()
		}
	}()
}
