package scheduler

import (
	"context"
	"sync"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"gorm.io/gorm"
)

type Scheduler struct {
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	db        *gorm.DB
	scrapeSvc *scraper.Service
	aggRepo   *reporting.AggregationRepository
}

func New() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Init(db *gorm.DB, scrapeSvc *scraper.Service, aggRepo *reporting.AggregationRepository) {
	s.db = db
	s.scrapeSvc = scrapeSvc
	s.aggRepo = aggRepo
}

func (s *Scheduler) Run(ctx context.Context) error {
	localCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	startScrape(localCtx, s.scrapeSvc, &s.wg)
	startStats(localCtx, s.db, s.aggRepo, &s.wg)

	<-localCtx.Done()
	s.wg.Wait()
	return nil
}

func (s *Scheduler) Shutdown() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}
