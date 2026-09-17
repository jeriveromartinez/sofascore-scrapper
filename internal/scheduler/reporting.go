package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/reporting"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const (
	lockStatsDaily     = "scheduler:lock:stats:daily"
	lockStatsMonthly   = "scheduler:lock:stats:monthly"
	lockUploadsCleanup = "scheduler:lock:uploads:cleanup"

	ttlStatsDaily     = 10 * time.Minute
	ttlStatsMonthly   = 30 * time.Minute
	ttlUploadsCleanup = 10 * time.Minute
)

func startStats(ctx context.Context, db interface{}, aggRepo interface{}, runner *Runner, cleanupJob *apk.CleanupJob, redisClient *redis.Client, counter apk.DownloadCounter, wg *sync.WaitGroup, logger *slog.Logger) {
	gormDB, ok := db.(*gorm.DB)
	if !ok || gormDB == nil {
		logger.Warn("scheduler: no DB available for stats")
		return
	}

	agg, ok := aggRepo.(*reporting.AggregationRepository)
	if !ok || agg == nil {
		logger.Warn("scheduler: no aggregation repository available")
	}

	_ = gormDB
	_ = agg

	c := cron.New()

	// Thread the parent ctx through every cron callback so SIGTERM
	// tears down jobs promptly instead of waiting for the lease TTL.
	// robfig/cron.Stop blocks until the running job returns, and the
	// previous context.Background() meant a job that happened to be
	// in-flight could stall shutdown by its full lock TTL.
	if agg != nil {
		_, err := c.AddFunc("1 0 * * *", func() {
			_ = runner.RunLocked(ctx, lockStatsDaily, ttlStatsDaily, func(jobCtx context.Context) error {
				if err := agg.GenerateDaily(); err != nil {
					logger.Error("failed to generate daily event stats", slog.String("error", err.Error()))
					return err
				}
				return nil
			})
		})
		if err != nil {
			logger.Error("failed to schedule daily stats cron job", slog.String("error", err.Error()))
		}

		_, err = c.AddFunc("10 0 1 * *", func() {
			_ = runner.RunLocked(ctx, lockStatsMonthly, ttlStatsMonthly, func(jobCtx context.Context) error {
				if err := agg.GenerateMonthly(); err != nil {
					logger.Error("failed to generate monthly event stats", slog.String("error", err.Error()))
					return err
				}
				return nil
			})
		})
		if err != nil {
			logger.Error("failed to schedule monthly stats cron job", slog.String("error", err.Error()))
		}
	}

	if cleanupJob != nil && redisClient != nil {
		_, err := c.AddFunc("*/15 * * * *", func() {
			_ = runner.RunLocked(ctx, lockUploadsCleanup, ttlUploadsCleanup, func(jobCtx context.Context) error {
				return cleanupJob.Run(jobCtx, redisClient)
			})
		})
		if err != nil {
			logger.Error("failed to schedule uploads cleanup cron job", slog.String("error", err.Error()))
		}
	}

	if counter != nil {
		_, err := c.AddFunc("*/15 * * * *", apkDownloadCounterJob(counter, logger, ctx))
		if err != nil {
			logger.Error("failed to schedule apk downloads flush cron job", slog.String("error", err.Error()))
		}
	}

	c.Start()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.Stop()
	}()
}

func apkDownloadCounterJob(counter apk.DownloadCounter, logger *slog.Logger, ctx context.Context) func() {
	return func() {
		if err := counter.Flush(ctx); err != nil {
			logger.Error("failed to flush apk download counters", slog.String("error", err.Error()))
			return
		}
		if err := counter.ReprocessOrphans(ctx); err != nil {
			logger.Error("failed to reprocess orphan apk download batches", slog.String("error", err.Error()))
		}
	}
}
