package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	lockEventReconcile = "scheduler:lock:events:reconcile"
	ttlEventReconcile  = 5 * time.Minute
)

// eventReconcileSpec runs the reconciler often enough that a zombie
// event leaves the table shortly after the read filter stops serving it.
var eventReconcileSpec = "@every 10m"

// staleLiveCloser is the subset of *events.Repository the reconciliation
// job needs, so tests can drive the cron with a counter.
type staleLiveCloser interface {
	CloseStaleLive(ctx context.Context, now time.Time) (int64, error)
}

func startEventReconcile(ctx context.Context, closer staleLiveCloser, runner *Runner, wg *sync.WaitGroup, logger *slog.Logger) {
	if closer == nil {
		if logger != nil {
			logger.Warn("scheduler: no events repository available, live event reconciliation disabled")
		}
		return
	}

	c := cron.New()

	if _, err := c.AddFunc(eventReconcileSpec, func() {
		runScheduledJob(ctx, runner, logger, "reconcile live events", lockEventReconcile, ttlEventReconcile, func(jobCtx context.Context) error {
			closed, err := closer.CloseStaleLive(jobCtx, time.Now())
			if err != nil {
				return err
			}
			if closed > 0 && logger != nil {
				logger.InfoContext(jobCtx, "scheduler: closed stale live events",
					slog.Int64("events", closed))
			}
			return nil
		})
	}); err != nil {
		if logger != nil {
			logger.Error("failed to schedule live event reconciliation cron job",
				slog.String("spec", eventReconcileSpec),
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
