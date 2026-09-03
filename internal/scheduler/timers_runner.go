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
)

// timersRunner is the per-timer scheduled-push dispatcher. Each
// tick it acquires a Redis distributed lock, then walks every
// pending timer (sourced from the DB) and dispatches the ones
// whose fire_at <= now. The Redis TimerStore is a fast index that
// the handler populates on schedule create and that the runner
// refreshes after each fire; if the store is nil or empty the
// runner still works because ListDueTimers queries the DB.
//
// Durability:
//   - The DB row in scheduled_push_timers is the source of truth;
//   Redis is just a fast index.
//   - On Redis unavailability the runner falls back to a DB scan
//   via the push service's ListDueTimers.
//   - MarkTimerDispatched is idempotent under worker races: two
//   runners processing the same timer both call into
//   DispatchTimer, but only the first one to set dispatched_at
//   succeeds; the loser exits without re-enqueuing or re-firing.
type timersRunner struct {
	svc        *push.Service
	store      redisplatform.TimerStore
	locker     redisplatform.Locker
	logger     *slog.Logger
	batchLimit int

	firedTotal  atomic.Int64
	failedTotal atomic.Int64
}

// startTimersRunner installs the timers runner as a background
// goroutine. It is safe to call with svc == nil; in that case the
// runner is a no-op (tests build an app without push sometimes).
// store may also be nil; the runner falls back to a DB scan.
func startTimersRunner(ctx context.Context, svc *push.Service, store redisplatform.TimerStore, tickInterval time.Duration, batchLimit int, locker redisplatform.Locker, logger *slog.Logger, wg *sync.WaitGroup) {
	if svc == nil {
		if logger != nil {
			logger.Warn("scheduler: no push service available, push schedules disabled")
		}
		return
	}
	if batchLimit <= 0 {
		batchLimit = 100
	}
	if tickInterval <= 0 {
		tickInterval = 5 * time.Second
	}
	r := &timersRunner{
		svc:        svc,
		store:      store,
		locker:     locker,
		logger:     logger,
		batchLimit: batchLimit,
	}
	r.start(ctx, tickInterval, wg)
	if logger != nil {
		logger.Info("timers runner scheduled",
			slog.Duration("interval", tickInterval),
			slog.Int("batch_limit", batchLimit))
	}
}

func (r *timersRunner) start(ctx context.Context, tickInterval time.Duration, wg *sync.WaitGroup) {
	ticker := time.NewTicker(tickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.tick(ctx)
			}
		}
	}()
}

const timersLockKey = "scheduler:lock:timers:run"
const timersLockTTL = 25 * time.Second

// tick is one iteration. Exported as RunOnce for the integration
// test so we can drive it deterministically.
func (r *timersRunner) tick(ctx context.Context) {
	lease, acquired, err := r.locker.Acquire(ctx, timersLockKey, timersLockTTL)
	if err != nil || !acquired {
		if err != nil {
			r.logger.Debug("timers runner: lock not acquired",
				slog.String("error", err.Error()))
		}
		return
	}
	defer func() {
		if relErr := lease.Release(ctx); relErr != nil {
			r.logger.Warn("timers runner: lock release failed",
				slog.String("error", relErr.Error()))
		}
	}()

	dueTimers, err := r.popDue(ctx)
	if err != nil {
		r.failedTotal.Add(1)
		r.logger.Error("timers runner: pop due",
			slog.String("error", err.Error()))
		return
	}
	for _, t := range dueTimers {
		if err := r.dispatchOne(ctx, t); err != nil {
			r.failedTotal.Add(1)
			r.logger.Error("timers runner: dispatch timer",
				slog.Uint64("timer_id", uint64(t.ID)),
				slog.String("error", err.Error()))
			continue
		}
		r.firedTotal.Add(1)
	}
}

func (r *timersRunner) popDue(ctx context.Context) ([]push.ScheduledPushTimer, error) {
	rows, err := r.svc.ListDueTimers(ctx, time.Now(), r.batchLimit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *timersRunner) dispatchOne(ctx context.Context, timer push.ScheduledPushTimer) error {
	sched, err := r.svc.LoadSchedule(ctx, timer.ScheduledPushID)
	if err != nil {
		return err
	}
	if err := r.svc.DispatchTimer(ctx, sched, timer); err != nil {
		return err
	}
	// Remove the timer from the Redis index after a successful
	// dispatch. If Redis is down we just skip — the next tick
	// will see dispatched_at is set in the DB and skip again.
	if r.store != nil {
		if err := r.store.Remove(ctx, sched.ID); err != nil {
			r.logger.Debug("timers runner: redis remove failed (continuing)",
				slog.Uint64("schedule_id", uint64(sched.ID)),
				slog.String("error", err.Error()))
		}
		// For recurring schedules the service has already
		// inserted a fresh DB row; re-enqueue it in Redis so
		// the next tick can find it via the fast path.
		if sched.ScheduleType == push.ScheduleTypeRecurring {
			if err := r.reEnqueueNext(ctx, sched.ID); err != nil {
				r.logger.Debug("timers runner: re-enqueue next failed (continuing)",
					slog.String("error", err.Error()))
			}
		}
	}
	return nil
}

// reEnqueueNext finds the earliest pending timer for the
// schedule and enqueues it into Redis. We only enqueue the next
// one (not all of them) because each subsequent fire will
// enqueue the next-next in turn. This keeps the ZSET lean.
func (r *timersRunner) reEnqueueNext(ctx context.Context, scheduleID uint) error {
	earliest, err := r.svc.EarliestPendingTimer(ctx, scheduleID)
	if err != nil || earliest.IsZero() {
		return err
	}
	entries, err := r.svc.PendingTimersAfter(ctx, scheduleID, earliest)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	return r.store.Enqueue(ctx, scheduleID, entries)
}

// firedCount and failedCount surface the atomic counters so the
// integration test can assert the runner did its work without
// scraping logs.
func (r *timersRunner) firedCount() int64  { return r.firedTotal.Load() }
func (r *timersRunner) failedCount() int64 { return r.failedTotal.Load() }

// RunOnce exposes the tick to the integration test so the cron
// clock is not in the loop.
func (r *timersRunner) RunOnce(ctx context.Context) {
	r.tick(ctx)
}

// ErrNoLocker is returned when the locker dependency is missing.
// Kept for symmetry with the other scheduler runners.
var ErrNoLocker = errors.New("scheduler: no Redis locker configured for timers runner")