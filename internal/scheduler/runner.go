package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
)

type Job func(context.Context) error

type Runner struct {
	locker redisplatform.Locker
}

func NewRunner(locker redisplatform.Locker) *Runner {
	return &Runner{locker: locker}
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
	var jobErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(ttl / 3)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if err := lease.Renew(jobCtx, ttl); err != nil {
					jobCancel()
					return
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer jobCancel()
		jobErr = job(jobCtx)
	}()

	wg.Wait()

	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer releaseCancel()
	_ = lease.Release(releaseCtx)

	return jobErr
}
