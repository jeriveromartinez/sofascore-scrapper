package events

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

const logoWorkerCount = 10

// LogoJob describes one team whose logo must be resolved. TeamName is
// optional but, when set, lets the downloader query TheSportsDB as a
// fallback when the primary URL fails (FotMob and SofaScore maintain
// independent team-ID spaces, so some teams have no asset in the
// SofaScore CDN).
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

func (s *LogoScheduler) Shutdown(ctx context.Context) {
	s.Stop()

	done := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.cancelWork()
	case <-ctx.Done():
		s.cancelWork()
		<-done
	}
}
