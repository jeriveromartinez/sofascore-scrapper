package scheduler

import (
	"context"
	"sync"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Scheduler struct {
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	db              *gorm.DB
	scrapeSvc       *scraper.Service
	aggRepo         *reporting.AggregationRepository
	runner          *Runner
	cleanupJob      *apk.CleanupJob
	redisClient     *redis.Client
	downloadCounter apk.DownloadCounter
}

func New() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Init(db *gorm.DB, scrapeSvc *scraper.Service, aggRepo *reporting.AggregationRepository, locker redisplatform.Locker) {
	s.db = db
	s.scrapeSvc = scrapeSvc
	s.aggRepo = aggRepo
	s.runner = NewRunner(locker)
}

func (s *Scheduler) SetCleanupJob(job *apk.CleanupJob, client *redis.Client) {
	s.cleanupJob = job
	s.redisClient = client
}

func (s *Scheduler) SetDownloadCounter(counter apk.DownloadCounter) {
	s.downloadCounter = counter
}

func (s *Scheduler) Run(ctx context.Context) error {
	localCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	startScrape(localCtx, s.scrapeSvc, s.runner, &s.wg)
	startStats(localCtx, s.db, s.aggRepo, s.runner, s.cleanupJob, s.redisClient, s.downloadCounter, &s.wg)

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
