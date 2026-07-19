package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

const (
	lockScrapeToday  = "scheduler:lock:scrape:today"
	lockScrapeFuture = "scheduler:lock:scrape:future"

	ttlScrapeToday  = 10 * time.Minute
	ttlScrapeFuture = 30 * time.Minute
)

func startScrape(ctx context.Context, svc *scraper.Service, runner *Runner, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = runner.RunLocked(ctx, lockScrapeToday, ttlScrapeToday, func(jobCtx context.Context) error {
			svc.ScrapeToday(jobCtx, time.Now())
			return nil
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = runner.RunLocked(ctx, lockScrapeFuture, ttlScrapeFuture, func(jobCtx context.Context) error {
			svc.ScrapeNext7Days(jobCtx)
			return nil
		})
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
				_ = runner.RunLocked(ctx, lockScrapeToday, ttlScrapeToday, func(jobCtx context.Context) error {
					return svc.Scrape(jobCtx, scraper.FOOTBALL, time.Now())
				})
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
				_ = runner.RunLocked(ctx, lockScrapeFuture, ttlScrapeFuture, func(jobCtx context.Context) error {
					svc.ScrapeNext7Days(jobCtx)
					return nil
				})
			}
		}
	}()
}
