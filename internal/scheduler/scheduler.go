package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	redisplatform "github.com/jeriveromartinez/scores365-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
	"github.com/jeriveromartinez/scores365-scrapper/internal/reporting"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Scheduler struct {
	// mu orders Run against Shutdown: they run on different goroutines
	// and every wg.Add must happen before Shutdown's wg.Wait.
	mu              sync.Mutex
	cancel          context.CancelFunc
	stopped         bool
	wg              sync.WaitGroup
	db              *gorm.DB
	scrapeSvc       *scraper.Service
	eventsCloser    staleLiveCloser
	aggRepo         *reporting.AggregationRepository
	runner          *Runner
	cleanupJob      *apk.CleanupJob
	redisClient     *redis.Client
	downloadCounter apk.DownloadCounter
	pushService     *push.Service
	pushLocker      redisplatform.Locker
	tickInterval    time.Duration
	batchLimit      int
	logger          *slog.Logger
}

func New(logger *slog.Logger) *Scheduler {
	return &Scheduler{logger: logger}
}

func (s *Scheduler) Init(db *gorm.DB, scrapeSvc *scraper.Service, aggRepo *reporting.AggregationRepository, locker redisplatform.Locker) {
	s.db = db
	s.scrapeSvc = scrapeSvc
	s.aggRepo = aggRepo
	if db != nil {
		s.eventsCloser = events.NewRepository(db)
	}
	s.runner = NewRunner(locker, s.logger)
}

func (s *Scheduler) SetCleanupJob(job *apk.CleanupJob, client *redis.Client) {
	s.cleanupJob = job
	s.redisClient = client
}

func (s *Scheduler) SetDownloadCounter(counter apk.DownloadCounter) {
	s.downloadCounter = counter
}

// SetPushService wires the push runner. svc and locker are
// required; passing nil for either disables the runner (used in
// tests that build an app without a push service).
func (s *Scheduler) SetPushService(svc *push.Service, locker redisplatform.Locker, tickInterval time.Duration, batchLimit int) {
	s.pushService = svc
	s.pushLocker = locker
	s.tickInterval = tickInterval
	s.batchLimit = batchLimit
}

// Run starts the jobs and blocks until ctx is cancelled or Shutdown is
// called. Run after Shutdown returns immediately without starting jobs.
func (s *Scheduler) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.cancel = cancel

	startScrape(localCtx, s.scrapeSvc, s.runner, &s.wg, s.logger)
	startEventReconcile(localCtx, s.eventsCloser, s.runner, &s.wg, s.logger)
	startStats(localCtx, s.db, s.aggRepo, s.runner, s.cleanupJob, s.redisClient, s.downloadCounter, &s.wg, s.logger)
	startTimersRunner(localCtx, s.pushService, s.tickInterval, s.batchLimit, s.pushLocker, s.logger, &s.wg)
	startPushGauges(localCtx, s.pushService, pushGaugesInterval, s.logger, &s.wg)
	s.mu.Unlock()

	<-localCtx.Done()
	s.wg.Wait()
	return nil
}

func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}
