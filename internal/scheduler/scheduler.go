package scheduler

import (
	"context"
)

type Scheduler struct {
	cancel context.CancelFunc
}

func New() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Start(ctx context.Context, db interface{}, scrapeSvc interface{}, aggRepo interface{}) {
	localCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	startScrape(localCtx, scrapeSvc)
	startStats(localCtx, db, aggRepo)
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
