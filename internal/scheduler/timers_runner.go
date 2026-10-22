package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	redisplatform "github.com/jeriveromartinez/scores365-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
)

// timersRunner is the per-timer scheduled-push dispatcher. Each
// tick it acquires a Redis distributed lock, then walks every
// pending timer (sourced from the DB) and dispatches the ones
// whose fire_at <= now.
//
// Durability:
//   - The DB row in scheduled_push_timers is the source of truth;
//     the runner reads it directly through ListDueTimers.
//   - MarkTimerDispatched is idempotent under worker races: two
//     runners processing the same timer both call into
//     DispatchTimer, but only the first one to set dispatched_at
//     succeeds; the loser exits without re-firing.
//   - A worker that dies between claiming a timer and completing the
//     dispatch leaves claimed_at set. The reaper releases such rows
//     once they age past staleClaimWindow, so the push is retried
//     instead of being lost forever.
//
// Mutual exclusion:
//   - Each tick holds a Redis lease. The lease is renewed while the
//     tick works, so a tick that runs longer than the lease TTL
//     cannot let a second instance take the same lock and dispatch
//     the same timers concurrently.
type timersRunner struct {
	svc              *push.Service
	locker           redisplatform.Locker
	logger           *slog.Logger
	batchLimit       int
	lockTTL          time.Duration
	staleClaimWindow time.Duration

	firedTotal  atomic.Int64
	failedTotal atomic.Int64
}

// startTimersRunner installs the timers runner as a background
// goroutine. It is safe to call with svc == nil; in that case the
// runner is a no-op (tests build an app without push sometimes).
// A nil locker disables the runner too: the tick needs the lease to
// dispatch a timer, and dereferencing a nil locker would panic in
// the background goroutine and take the process down.
func startTimersRunner(ctx context.Context, svc *push.Service, tickInterval time.Duration, batchLimit int, locker redisplatform.Locker, logger *slog.Logger, wg *sync.WaitGroup) {
	if logger == nil {
		logger = slog.Default()
	}
	if svc == nil {
		logger.Warn("scheduler: no push service available, push schedules disabled")
		return
	}
	if locker == nil {
		logger.Warn("scheduler: no Redis locker available, push schedules disabled",
			slog.String("error", ErrNoLocker.Error()))
		return
	}
	if batchLimit <= 0 {
		batchLimit = 100
	}
	if tickInterval <= 0 {
		tickInterval = 5 * time.Second
	}
	r := &timersRunner{
		svc:              svc,
		locker:           locker,
		logger:           logger,
		batchLimit:       batchLimit,
		lockTTL:          timersLockTTL,
		staleClaimWindow: timersStaleClaimWindow,
	}
	r.start(ctx, tickInterval, wg)
	logger.Info("timers runner scheduled",
		slog.Duration("interval", tickInterval),
		slog.Int("batch_limit", batchLimit))
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

// timersTickTimeout bounds one tick so a hung query cannot stall
// scheduled pushes forever. It stays below timersLockTTL so a stuck tick
// gives up before its lease would need to outlive the TTL.
const timersTickTimeout = 20 * time.Second

// timersStaleClaimWindow is how long a timer claim may stay open
// before the reaper assumes the claiming worker died mid-dispatch.
//
// It must be far larger than the worst-case duration of a single
// dispatch (a handful of DB writes plus one websocket publish), so a
// slow-but-alive worker never has its claim stolen. 5 minutes gives
// a wide margin over the sub-second dispatch path while keeping the
// recovery delay short enough that a lost push is retried promptly.
const timersStaleClaimWindow = 5 * time.Minute

// claimWindow is the reaper's cutoff window. It is overridable so
// tests can drive the reaper with a shorter window.
func (r *timersRunner) claimWindow() time.Duration {
	if r.staleClaimWindow > 0 {
		return r.staleClaimWindow
	}
	return timersStaleClaimWindow
}

// leaseTTL is the TTL of the Redis lease held during a tick. It is
// overridable so tests can drive a tick that outruns the lease.
func (r *timersRunner) leaseTTL() time.Duration {
	if r.lockTTL > 0 {
		return r.lockTTL
	}
	return timersLockTTL
}

// log returns the runner's logger, falling back to the default
// handler so a directly constructed runner cannot panic on a nil
// logger.
func (r *timersRunner) log() *slog.Logger {
	if r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

// tick is one iteration. Exported as RunOnce for the integration
// test so we can drive it deterministically.
//
// The tick runs under a Redis lease that is renewed while the tick
// works. Without the renewal a tick that dispatches more than the
// lease TTL worth of timers would let the lease expire: a second
// instance would then acquire the same key and dispatch the same
// timers concurrently.
func (r *timersRunner) tick(ctx context.Context) {
	if r.locker == nil {
		r.log().Warn("timers runner: no Redis locker available, push schedules disabled",
			slog.String("error", ErrNoLocker.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timersTickTimeout)
	defer cancel()

	runner := NewRunner(r.locker, r.log())
	err := runner.RunLocked(ctx, timersLockKey, r.leaseTTL(), func(jobCtx context.Context) error {
		r.dispatchDue(jobCtx)
		return nil
	})
	if err != nil {
		r.log().Warn("timers runner: tick aborted",
			slog.String("error", err.Error()))
	}
}

// dispatchDue pops the timers that are due and dispatches them.
// It runs while the tick holds the lease.
func (r *timersRunner) dispatchDue(ctx context.Context) {
	r.reapStaleClaims(ctx)
	dueTimers, err := r.popDue(ctx)
	if err != nil {
		r.failedTotal.Add(1)
		r.log().Error("timers runner: pop due",
			slog.String("error", err.Error()))
		return
	}
	for _, t := range dueTimers {
		if err := r.dispatchOne(ctx, t); err != nil {
			r.failedTotal.Add(1)
			r.log().Error("timers runner: dispatch timer",
				slog.Uint64("timer_id", uint64(t.ID)),
				slog.String("error", err.Error()))
			continue
		}
		r.firedTotal.Add(1)
	}
}

// reapStaleClaims releases the timers whose claim is still open past
// the stale window: the worker that claimed them died before
// completing the dispatch, so the push would otherwise never fire.
// Rows whose dispatch completed are left alone.
func (r *timersRunner) reapStaleClaims(ctx context.Context) {
	cutoff := time.Now().Add(-r.claimWindow())
	released, err := r.svc.ReleaseStaleTimerClaims(ctx, cutoff)
	if err != nil {
		r.log().Warn("timers runner: reap stale timer claims",
			slog.String("error", err.Error()))
		return
	}
	if released > 0 {
		r.log().Info("timers runner: released stale timer claims",
			slog.Int64("released", released))
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
	return r.svc.DispatchTimer(ctx, sched, timer)
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
