package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/push"
	"github.com/robfig/cron/v3"
)

// pushRunner is the cron-driven consumer that fires due
// scheduled_pushes. It runs every 30s (configurable via the cron
// spec) and uses a Redis distributed lock so only one backend in
// the cluster dispatches on any given tick. Without the lock,
// every backend would fire every schedule, multiplying the
// delivery_attempts rows by N.
type pushRunner struct {
	svc    *push.Service
	locker redisplatform.Locker
	logger *slog.Logger

	// Counters kept in-process for the lifetime of the runner
	// (one per backend). Exposed via the metrics layer in a
	// follow-up; for now they surface as logs.
	firedTotal  atomic.Int64
	failedTotal atomic.Int64
}

// startPushSchedules installs the push runner as a background
// goroutine alongside the existing scrape + stats workers. It is
// safe to call with svc == nil; in that case the runner is a
// no-op and a warning is logged (the tests that build an app
// without a push service do not get a panic).
func startPushSchedules(ctx context.Context, svc *push.Service, locker redisplatform.Locker, logger *slog.Logger, wg *sync.WaitGroup) {
	if svc == nil {
		if logger != nil {
			logger.Warn("scheduler: no push service available, push schedules disabled")
		}
		return
	}
	if locker == nil {
		if logger != nil {
			logger.Warn("scheduler: no Redis locker available, push schedules disabled")
		}
		return
	}
	r := &pushRunner{svc: svc, locker: locker, logger: logger}

	const spec = "*/30 * * * * *" // every 30 seconds; 6-field spec requires WithSeconds
	c := cron.New(cron.WithSeconds())
	if _, err := c.AddFunc(spec, func() {
		r.tick(ctx)
	}); err != nil {
		if logger != nil {
			logger.Error("failed to schedule push runner",
				slog.String("spec", spec),
				slog.String("error", err.Error()))
		}
		return
	}
	c.Start()
	if logger != nil {
		logger.Info("push runner scheduled", slog.String("spec", spec))
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.Stop()
	}()
}

// lockTTL bounds how long a single backend can hold the runner
// lock. Slightly shorter than the 30s tick interval so the lock
// is released before the next tick can race for it. The TTL is
// also long enough to absorb a slow tick (e.g. under load) without
// the lock expiring mid-dispatch.
const pushRunLockKey = "scheduler:lock:push:run"
const pushRunLockTTL = 25 * time.Second

// tick is one iteration of the runner. It is exported as
// RunOnce so the integration test can drive it deterministically
// (no waiting on the cron clock).
func (r *pushRunner) tick(ctx context.Context) {
	// Distributed lock so only one backend in the cluster runs the
	// dispatch. Acquire returns (lease, acquired, err); if
	// acquired is false the lock is held by another instance and
	// we silently skip (the next tick will try again).
	lease, acquired, err := r.locker.Acquire(ctx, pushRunLockKey, pushRunLockTTL)
	if err != nil || !acquired {
		if err != nil {
			r.logger.Debug("push runner: lock not acquired",
				slog.String("key", pushRunLockKey),
				slog.String("error", err.Error()))
		}
		return
	}
	defer func() {
		if relErr := lease.Release(ctx); relErr != nil {
			r.logger.Warn("push runner: lock release failed",
				slog.String("key", pushRunLockKey),
				slog.String("error", relErr.Error()))
		}
	}()

	schedules, err := r.svc.ListDueScheduledPushes(ctx, time.Now(), 100)
	if err != nil {
		r.failedTotal.Add(1)
		r.logger.Error("push runner: list due",
			slog.String("error", err.Error()))
		return
	}
	for i := range schedules {
		if err := r.svc.DispatchScheduled(ctx, &schedules[i]); err != nil {
			r.failedTotal.Add(1)
			r.logger.Error("push runner: dispatch",
				slog.Uint64("schedule_id", uint64(schedules[i].ID)),
				slog.String("error", err.Error()))
			// MarkScheduledPushFired has already been called inside
			// DispatchScheduled (best-effort), so a transient DB
			// error does not leave a row perpetually stuck as "due".
			// The next tick will pick it up again if the
			// reschedule did not commit; for one_shot we move on.
			continue
		}
		r.firedTotal.Add(1)
	}
}

// RunOnce exposes the tick to the integration test so the cron
// clock is not in the loop. It is a thin wrapper around
// (*pushRunner).tick, exported for tests.
func (r *pushRunner) RunOnce(ctx context.Context) {
	r.tick(ctx)
}

// errorsForTest surfaces the atomic counters so the integration
// test can assert the runner fired the expected number of
// schedules without scraping logs.
func (r *pushRunner) firedCount() int64  { return r.firedTotal.Load() }
func (r *pushRunner) failedCount() int64 { return r.failedTotal.Load() }

// ErrNoLocker is returned when the locker dependency is missing.
// Kept for symmetry with the other scheduler runners; the runner
// just logs and exits in that case.
var ErrNoLocker = errors.New("scheduler: no Redis locker configured for push runner")
