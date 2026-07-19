package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

func startScrape(ctx context.Context, svc *scraper.Service, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		svc.ScrapeToday(ctx, time.Now())
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		svc.ScrapeNext7Days(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				svc.Scrape(ctx, scraper.FOOTBALL, time.Now())
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
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
				svc.ScrapeNext7Days(ctx)
			}
		}
	}()
}
