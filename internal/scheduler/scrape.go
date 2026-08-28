package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	lockScrapeToday  = "scheduler:lock:scrape:today"
	lockScrapeFuture = "scheduler:lock:scrape:future"

	// TTLs are sized for the job, not for safety against crashes.
	// The previous values (10m / 30m) caused scrape starvation when
	// multiple backend instances contended for the same lock; the
	// winning instance held the lock for up to 10 minutes, during
	// which every other instance's tick was skipped.
	ttlScrapeToday  = 5 * time.Minute
	ttlScrapeFuture = 25 * time.Minute
)

var (
	// Production cron specs. Tests override these with shorter cadences
	// to keep wall-clock time under control.
	scrapeTodaySpec  = "@every 1m"
	scrapeFutureSpec = "0 6,18 * * *"
)

// scraperService is the subset of *scraper.Service that startScrape
// needs. It exists so tests can inject a counting mock without standing
// up a full scraper stack.
type scraperService interface {
	ScrapeToday(ctx context.Context, date time.Time)
	ScrapeNext7Days(ctx context.Context)
}

func startScrape(ctx context.Context, svc scraperService, runner *Runner, wg *sync.WaitGroup, logger *slog.Logger) {
	if svc == nil {
		if logger != nil {
			logger.Warn("scheduler: no scraper service available, scrape jobs disabled")
		}
		return
	}

	c := cron.New()

	if _, err := c.AddFunc(scrapeTodaySpec, func() {
		_ = runner.RunLocked(context.Background(), lockScrapeToday, ttlScrapeToday, func(jobCtx context.Context) error {
			svc.ScrapeToday(jobCtx, time.Now())
			return nil
		})
	}); err != nil {
		if logger != nil {
			logger.Error("failed to schedule scrape-today cron job",
				slog.String("spec", scrapeTodaySpec),
				slog.String("error", err.Error()))
		}
		return
	}

	if _, err := c.AddFunc(scrapeFutureSpec, func() {
		_ = runner.RunLocked(context.Background(), lockScrapeFuture, ttlScrapeFuture, func(jobCtx context.Context) error {
			svc.ScrapeNext7Days(jobCtx)
			return nil
		})
	}); err != nil {
		if logger != nil {
			logger.Error("failed to schedule scrape-future cron job",
				slog.String("spec", scrapeFutureSpec),
				slog.String("error", err.Error()))
		}
		return
	}

	c.Start()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.Stop()
	}()
}
