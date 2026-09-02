package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/push"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scheduler"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/seeder"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

type App struct {
	HTTP          *http.Server
	Pprof         *http.Server
	Scheduler     *scheduler.Scheduler
	DB            *gorm.DB
	SQL           *sql.DB
	Redis         *goredis.Client
	logoScheduler events.TeamLogoScheduler
	// realtimeHub is the local WebSocket registry. One per backend
	// instance; the Redis pub/sub subscriber fans out cross-instance.
	realtimeHub *realtime.Hub
	// realtimeSubscriber consumes push:fanout and dispatches to the
	// local hub. Stopped during shutdown.
	realtimeSubscriber *realtime.Subscriber
	// pushService is the push-notifications domain service. It is
	// shared with the realtime WS handler (the AckHandler closure
	// installed in routes.go calls pushService.OnAck) so acks
	// flip delivery_attempts to DELIVERED on the same database
	// the REST handlers read.
	pushService *push.Service
	// pushMetrics exports the push counters/gauges/histograms to
	// the project's Prometheus registry. nil-safe so tests can
	// skip wiring it.
	pushMetrics *push.Metrics
	ready       atomic.Bool
	batchSize   int
	concur      int
	storagePath string
	logger      *slog.Logger
}

func (a *App) IsReady() bool {
	return a.ready.Load()
}

func New(cfg config.Config) (*App, error) {
	logger := observability.NewLogger(slog.LevelInfo, os.Stdout)
	slog.SetDefault(logger)

	tokens, err := auth.NewTokenService(cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		return nil, err
	}

	if !cfg.SkipMigrate {
		if err := database.AutoMigrateAll(db); err != nil {
			return nil, fmt.Errorf("automigrate: %w", err)
		}
		if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
			return nil, fmt.Errorf("seed default admin: %w", err)
		}
	}

	redisClient, err := redisplatform.New(context.Background(), cfg.Redis)
	if err != nil {
		return nil, err
	}

	if err := observability.Register(server.NewSQLCollector(sqlDB)); err != nil {
		return nil, err
	}
	if err := observability.Register(server.NewRedisCollector(redisClient)); err != nil {
		return nil, err
	}

	sched := scheduler.New(slog.Default())
	logoScheduler := events.NewLogoScheduler()

	pprofSrv := observability.NewPprofServer(cfg.PprofAddr, cfg.PprofEnabled)

	app := &App{
		Scheduler:     sched,
		DB:            db,
		SQL:           sqlDB,
		Redis:         redisClient,
		logoScheduler: logoScheduler,
		Pprof:         pprofSrv,
		realtimeHub:   realtime.NewHub(),
		batchSize:     cfg.ScrapeBatchSize,
		concur:        cfg.ScrapeConcurrency,
		storagePath:   cfg.APKStoragePath,
		logger:        logger,
	}
	app.realtimeSubscriber = realtime.NewSubscriber(redisClient, app.realtimeHub, realtime.SubscriberConfig{
		Logger: logger,
	})
	pushMetrics := push.NewMetrics(prometheus.DefaultRegisterer)
	app.pushMetrics = pushMetrics
	pushRepo := push.NewRepository(db)
	domainRepoPush := domains.NewRepository(db)
	userRepoPush := users.NewRepository(db)
	app.pushService = push.NewService(
		pushRepo,
		push.NewRealtimeSubscriberPusher(app.realtimeSubscriber),
		domainRepoPush,
		userRepoPush,
		logger,
	)
	app.pushService.SetMetrics(pushMetrics)
	app.realtimeHub.SetMetrics(pushMetrics)
	app.ready.Store(true)

	router := NewRouter(db, redisClient, cfg, tokens, logoScheduler, app.realtimeHub, app.pushService)
	router.Use(server.ReadinessMiddleware(&app.ready))

	app.HTTP = &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	if a.DB != nil {
		if err := events.NewRepositoryWithLogoScheduler(a.DB, a.logoScheduler).ReconcileTeamLogos(ctx); err != nil {
			return err
		}
	}

	scrapeSvc, aggRepo := buildSchedulerDeps(a.DB, a.batchSize, a.concur, events.NewEpochStore(a.Redis), a.logoScheduler, a.logger)
	a.Scheduler.Init(a.DB, scrapeSvc, aggRepo, redisplatform.NewLocker(a.Redis))
	a.Scheduler.SetCleanupJob(buildCleanupJobFromApp(a), a.Redis)
	a.Scheduler.SetDownloadCounter(apk.NewDownloadCounter(a.Redis, a.DB))
	a.Scheduler.SetPushService(a.pushService, redisplatform.NewLocker(a.Redis))

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if a.realtimeSubscriber == nil {
			// Tests construct an App directly without wiring the
			// subscriber; treat this as a no-op rather than a panic.
			<-ctx.Done()
			return nil
		}
		if err := a.realtimeSubscriber.Start(ctx); err != nil {
			a.logger.Error("realtime: subscriber start failed", slog.String("error", err.Error()))
			return err
		}
		return nil
	})
	group.Go(func() error {
		a.logger.Info("API server listening", slog.String("addr", a.HTTP.Addr))
		return a.HTTP.ListenAndServe()
	})
	if a.Pprof != nil {
		group.Go(func() error {
			a.logger.Info("pprof server listening", slog.String("addr", a.Pprof.Addr))
			return observability.PprofListenAndServe(a.Pprof)
		})
	}
	group.Go(func() error { return a.Scheduler.Run(ctx) })
	group.Go(func() error {
		<-ctx.Done()
		return a.shutdown()
	})
	return normalizeServerClosed(group.Wait())
}
