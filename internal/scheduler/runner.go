package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
	redisplatform "github.com/jeriveromartinez/scores365-scrapper/internal/platform/redis"
)

type Job func(context.Context) error

type Runner struct {
	locker redisplatform.Locker
	logger *slog.Logger
}

func NewRunner(locker redisplatform.Locker, logger *slog.Logger) *Runner {
	return &Runner{locker: locker, logger: logger}
}

func (r *Runner) RunLocked(ctx context.Context, name string, ttl time.Duration, job Job) error {
	if r.locker == nil {
		return runMeasured(ctx, name, job)
	}

	lease, acquired, err := r.locker.Acquire(ctx, name, ttl)
	if err != nil {
		return fmt.Errorf("acquire lock %s: %w", name, err)
	}
	if !acquired {
		observability.SchedulerLockContention.WithLabelValues(name).Inc()
		return nil
	}

	jobCtx, jobCancel := context.WithCancel(ctx)
	defer jobCancel()

	var wg sync.WaitGroup
	var jobErr, renewErr error

	renewInterval := ttl / 3
	if renewInterval <= 0 {
		renewInterval = ttl
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if err := lease.Renew(jobCtx, ttl); err != nil {
					if jobCtx.Err() == nil {
						renewErr = err
					}
					jobCancel()
					return
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		jobErr = runMeasured(jobCtx, name, job)
		// The renewal goroutine only exits when jobCtx is cancelled,
		// and jobCancel used to be deferred until after wg.Wait(),
		// so the wait never returned and the lock was renewed
		// forever. Cancel here so the renewal goroutine stops.
		jobCancel()
	}()

	wg.Wait()

	releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer releaseCancel()
	if relErr := lease.Release(releaseCtx); relErr != nil && r.logger != nil {
		r.logger.Warn("scheduler: lock release failed",
			slog.String("lock", name),
			slog.String("error", relErr.Error()))
	}

	if renewErr != nil {
		return fmt.Errorf("renew lock %s: %w", name, renewErr)
	}
	return jobErr
}

// runScheduledJob runs a job under the named lease and reports a
// failure through slog. The cron callbacks use it because RunLocked
// reports lease failures (Redis down, lease lost) that the jobs
// themselves cannot see.
func runScheduledJob(ctx context.Context, runner *Runner, logger *slog.Logger, job, lock string, ttl time.Duration, fn Job) {
	err := runner.RunLocked(ctx, lock, ttl, fn)
	if err == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("scheduler: job failed",
		slog.String("job", job),
		slog.String("lock", lock),
		slog.String("error", err.Error()))
}

func runMeasured(ctx context.Context, name string, job Job) error {
	start := time.Now()
	err := job(ctx)
	observability.SchedulerJobRuns.WithLabelValues(name).Inc()
	observability.SchedulerJobDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())
	return err
}
