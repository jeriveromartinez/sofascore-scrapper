package events

import "sync"

const logoWorkerCount = 10

type logoScheduler struct {
	mu       sync.Mutex
	ready    *sync.Cond
	pending  map[int64]func()
	queue    []int64
	head     int
	stopping bool
	workers  sync.WaitGroup
}

func newLogoScheduler(workerCount int) *logoScheduler {
	scheduler := &logoScheduler{pending: make(map[int64]func())}
	scheduler.ready = sync.NewCond(&scheduler.mu)
	scheduler.workers.Add(workerCount)
	for range workerCount {
		go scheduler.run()
	}
	return scheduler
}

func (s *logoScheduler) enqueue(teamID int64, work func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopping {
		return
	}
	if _, exists := s.pending[teamID]; exists {
		return
	}

	s.pending[teamID] = work
	s.queue = append(s.queue, teamID)
	s.ready.Signal()
}

func (s *logoScheduler) run() {
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

		work()

		s.mu.Lock()
		delete(s.pending, teamID)
		s.mu.Unlock()
	}
}

func (s *logoScheduler) shutdown() {
	s.mu.Lock()
	s.stopping = true
	s.ready.Broadcast()
	s.mu.Unlock()
	s.workers.Wait()
}
