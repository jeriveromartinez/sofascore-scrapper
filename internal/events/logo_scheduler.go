package events

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

const logoWorkerCount = 10

// LogoJob describes one team whose logo must be resolved. TeamName is
// optional but, when set, lets the downloader consult an installed
// LogoLookup as a fallback when the primary URL fails (FotMob and
// 365scores maintain independent team-ID spaces, so an ID-derived
// badge URL only resolves on the CDN that owns that ID).
type LogoJob struct {
	TeamID     int64
	TeamName   string
	PrimaryURL string
}

// LogoJobHandler is the per-job callback the Repository installs at
// scheduler construction time. The scheduler hands the work to a
// worker pool which calls the handler on a separate goroutine; the
// handler must be safe to call concurrently.
type LogoJobHandler func(ctx context.Context, db *gorm.DB, job LogoJob)

type TeamLogoScheduler interface {
	Schedule(*gorm.DB, LogoJob)
	Stop()
	Shutdown(context.Context)
}

type LogoScheduler struct {
	mu         sync.Mutex
	ready      *sync.Cond
	pending    map[int64]func(context.Context)
	queue      []int64
	head       int
	stopping   bool
	workCtx    context.Context
	cancelWork context.CancelFunc
	workers    sync.WaitGroup
	handler    LogoJobHandler
}

// NewLogoScheduler builds a scheduler that delegates each job to
// handler. handler is invoked from the worker pool — callers must
// keep it free of long-blocking work that the scheduler's Stop /
// Shutdown cannot cancel via context.
func NewLogoScheduler(handler LogoJobHandler) *LogoScheduler {
	return newLogoScheduler(logoWorkerCount, handler)
}

func newLogoScheduler(workerCount int, handler LogoJobHandler) *LogoScheduler {
	workCtx, cancelWork := context.WithCancel(context.Background())
	scheduler := &LogoScheduler{
		pending:    make(map[int64]func(context.Context)),
		workCtx:    workCtx,
		cancelWork: cancelWork,
		handler:    handler,
	}
	scheduler.ready = sync.NewCond(&scheduler.mu)
	scheduler.workers.Add(workerCount)
	for range workerCount {
		go scheduler.run()
	}
	return scheduler
}

func (s *LogoScheduler) enqueue(teamID int64, work func(context.Context)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopping {
		return false
	}
	if _, exists := s.pending[teamID]; exists {
		return true
	}

	s.pending[teamID] = work
	s.queue = append(s.queue, teamID)
	s.ready.Signal()
	return true
}

func (s *LogoScheduler) Schedule(db *gorm.DB, job LogoJob) {
	if s.handler == nil {
		// Defensive default: tests can construct a scheduler without
		// a handler to exercise queue / dedup behavior. Production
		// always passes a handler via NewLogoScheduler.
		s.enqueue(job.TeamID, func(context.Context) {})
		return
	}
	s.enqueue(job.TeamID, func(ctx context.Context) {
		s.handler(ctx, db.Session(&gorm.Session{}), job)
	})
}

func (s *LogoScheduler) run() {
	defer s.workers.Done()
	for {
		s.mu.Lock()
		for s.head == len(s.queue) && !s.stopping {
			s.ready.Wait()
		}
		if s.head == len(s.queue) {
			s.mu.Unlock()
			return
		}

		teamID := s.queue[s.head]
		s.head++
		work := s.pending[teamID]
		// Retain the pending entry while active so concurrent duplicates coalesce.
		if s.head == len(s.queue) {
			s.queue = nil
			s.head = 0
		}
		s.mu.Unlock()

		work(s.workCtx)

		s.mu.Lock()
		delete(s.pending, teamID)
		s.mu.Unlock()
	}
}

func (s *LogoScheduler) Stop() {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	for _, teamID := range s.queue[s.head:] {
		delete(s.pending, teamID)
	}
	s.queue = nil
	s.head = 0
	s.ready.Broadcast()
	s.mu.Unlock()
}

// Shutdown stops accepting new work, waits for in-flight handlers
// to finish (or for ctx to expire, whichever comes first), then
// releases s.workCtx.
//
// The happy path (workers finish naturally) does NOT cancel
// s.workCtx until after the workers have drained — there is nothing
// left to cancel, and signalling a Done on a context with no
// consumers is pointless. Only the deadline path cancels in-flight
// handlers, because by definition they have not finished and need
// to be told to abort.
//
// We always wait for workers.Wait() to return before exiting, even
// in the deadline branch. Returning while a worker is still
// unwinding its cancel path would leak goroutines and leave the
// scheduler in a half-closed state if a caller invokes Shutdown a
// second time.
func (s *LogoScheduler) Shutdown(ctx context.Context) {
	s.Stop()

	done := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Workers finished before the deadline. Nothing to cancel;
		// s.workCtx had no remaining consumers. Release it
		// anyway so the cancel goroutine it carries can exit
		// cleanly (cheap, idempotent if the deadline branch
		// already fired).
		s.cancelWork()
	case <-ctx.Done():
		// Deadline hit while workers were mid-download. Signal
		// s.workCtx so the handler can abort its HTTP round-trip
		// or disk write, then wait for the worker goroutines to
		// drain before returning.
		s.cancelWork()
		<-done
	}
}
