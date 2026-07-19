package scheduler

import (
	"context"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

func scrape(svc *scraper.Service, sport string, date time.Time) {
	svc.Scrape(sport, date)
}

func scrapeCountry(svc *scraper.Service, countryCode string) {
	svc.ScrapeCountry(countryCode)
}

func scrapeToday(svc *scraper.Service, date time.Time) {
	for _, sport := range scraper.GET_SPORTS() {
		scrape(svc, sport, date)
	}
	for _, country := range scraper.GET_COUNTRIES() {
		scrapeCountry(svc, country)
	}
}

func scrapeNext7Days(svc *scraper.Service) {
	now := time.Now()
	for _, sport := range scraper.GET_SPORTS() {
		for i := 1; i <= 7; i++ {
			scrape(svc, sport, now.Add(time.Duration(i)*24*time.Hour))
		}
	}
}

func startScrape(ctx context.Context, scrapeSvc interface{}) {
	svc, ok := scrapeSvc.(*scraper.Service)
	if !ok || svc == nil {
		return
	}

	go scrapeToday(svc, time.Now())
	go scrapeNext7Days(svc)

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scrape(svc, scraper.FOOTBALL, time.Now())
			}
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

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				scrapeNext7Days(svc)
			}
		}
	}()
}
