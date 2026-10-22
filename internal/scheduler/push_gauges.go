package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
)

// pushGaugesInterval is how often the process-wide push gauges are
// recomputed from the database. The counts only move when a user
// flips the notifications toggle or a schedule is created, paused or
// removed, so a minute is frequent enough for the dashboard and cheap
// enough to run two COUNT queries.
const pushGaugesInterval = time.Minute

// startPushGauges launches the background goroutine that keeps
// push_enabled_users and push_active_schedules in sync with the
// database.
//
// It runs outside the timers dispatch lease on purpose: every
// instance owns its own Prometheus registry, so each one refreshes
// its own gauges instead of waiting for an elected writer.
func startPushGauges(ctx context.Context, svc *push.Service, interval time.Duration, logger *slog.Logger, wg *sync.WaitGroup) {
	if logger == nil {
		logger = slog.Default()
	}
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = pushGaugesInterval
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := svc.RefreshGauges(ctx); err != nil {
				logger.Warn("scheduler: refresh push gauges failed",
					slog.String("error", err.Error()))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
