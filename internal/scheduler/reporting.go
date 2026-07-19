package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const (
	lockStatsDaily      = "scheduler:lock:stats:daily"
	lockStatsMonthly    = "scheduler:lock:stats:monthly"
	lockUploadsCleanup  = "scheduler:lock:uploads:cleanup"
	lockApkDownloadsFlush = "scheduler:lock:apk-downloads:flush"

	ttlStatsDaily      = 10 * time.Minute
	ttlStatsMonthly    = 30 * time.Minute
	ttlUploadsCleanup  = 10 * time.Minute
	ttlApkDownloadsFlush = 10 * time.Minute
)

func startStats(ctx context.Context, db interface{}, aggRepo interface{}, runner *Runner, wg *sync.WaitGroup) {
	gormDB, ok := db.(*gorm.DB)
	if !ok || gormDB == nil {
		log.Printf("scheduler: no DB available for stats")
		return
	}

	agg, ok := aggRepo.(*reporting.AggregationRepository)
	if !ok || agg == nil {
		return
	}

	_ = gormDB

	c := cron.New()

	_, err := c.AddFunc("1 0 * * *", func() {
		_ = runner.RunLocked(context.Background(), lockStatsDaily, ttlStatsDaily, func(jobCtx context.Context) error {
			if err := agg.GenerateDaily(); err != nil {
				log.Printf("failed to generate daily event stats: %v", err)
				return err
			}
			return nil
		})
	})
	if err != nil {
		log.Printf("failed to schedule daily stats cron job: %v", err)
	}

	_, err = c.AddFunc("10 0 1 * *", func() {
		_ = runner.RunLocked(context.Background(), lockStatsMonthly, ttlStatsMonthly, func(jobCtx context.Context) error {
			if err := agg.GenerateMonthly(); err != nil {
				log.Printf("failed to generate monthly event stats: %v", err)
				return err
			}
			return nil
		})
	})
	if err != nil {
		log.Printf("failed to schedule monthly stats cron job: %v", err)
	}

	_, err = c.AddFunc("0 3 * * *", func() {
		_ = runner.RunLocked(context.Background(), lockUploadsCleanup, ttlUploadsCleanup, func(jobCtx context.Context) error {
			return nil
		})
	})
	if err != nil {
		log.Printf("failed to schedule uploads cleanup cron job: %v", err)
	}

	_, err = c.AddFunc("*/15 * * * *", func() {
		_ = runner.RunLocked(context.Background(), lockApkDownloadsFlush, ttlApkDownloadsFlush, func(jobCtx context.Context) error {
			return nil
		})
	})
	if err != nil {
		log.Printf("failed to schedule apk downloads flush cron job: %v", err)
	}

	c.Start()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.Stop()
	}()
}
