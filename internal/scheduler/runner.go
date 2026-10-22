package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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
		return job(ctx)
	}

	lease, acquired, err := r.locker.Acquire(ctx, name, ttl)
	if err != nil {
		return fmt.Errorf("acquire lock %s: %w", name, err)
	}
	if !acquired {
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
		jobErr = job(jobCtx)
		// The renewal goroutine only exits when jobCtx is cancelled,
		// and jobCancel used to be deferred until after wg.Wait(),
		// so the wait never returned and the lock was renewed
		// forever. Cancel here so the renewal goroutine stops.
		jobCancel()
	}()

	wg.Wait()

	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
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
