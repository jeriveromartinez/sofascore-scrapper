package events

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

const logoWorkerCount = 10

type TeamLogoScheduler interface {
	Schedule(*gorm.DB, int64, string)
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
}

func NewLogoScheduler() *LogoScheduler {
	return newLogoScheduler(logoWorkerCount)
}

func newLogoScheduler(workerCount int) *LogoScheduler {
	workCtx, cancelWork := context.WithCancel(context.Background())
	scheduler := &LogoScheduler{
		pending:    make(map[int64]func(context.Context)),
		workCtx:    workCtx,
		cancelWork: cancelWork,
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

func (s *LogoScheduler) Schedule(db *gorm.DB, teamID int64, sourceURL string) {
	s.enqueue(teamID, func(ctx context.Context) {
		downloadAndUpdateTeamLogo(ctx, db.Session(&gorm.Session{}), teamID, sourceURL)
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
