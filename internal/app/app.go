package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scheduler"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

type App struct {
	HTTP      *http.Server
	Scheduler *scheduler.Scheduler
	DB        *gorm.DB
	SQL       *sql.DB
	Redis     *goredis.Client
	ready     atomic.Bool
	batchSize int
	concur    int
}

func (a *App) IsReady() bool {
	return a.ready.Load()
}

func New(cfg config.Config) (*App, error) {
	tokens, err := auth.NewTokenService(cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		return nil, err
	}

	if err := database.Migrate(context.Background(), sqlDB); err != nil {
		return nil, err
	}

	redisClient, err := redisplatform.New(context.Background(), cfg.Redis)
	if err != nil {
		return nil, err
	}

	sched := scheduler.New()

	app := &App{
		Scheduler: sched,
		DB:        db,
		SQL:       sqlDB,
		Redis:     redisClient,
		batchSize: cfg.ScrapeBatchSize,
		concur:    cfg.ScrapeConcurrency,
	}
	app.ready.Store(true)

	router := NewRouter(db, redisClient, cfg, tokens)
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
	scrapeSvc, aggRepo := buildSchedulerDeps(a.DB, a.batchSize, a.concur)
	a.Scheduler.Init(a.DB, scrapeSvc, aggRepo)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		log.Printf("API server listening on %s", a.HTTP.Addr)
		return a.HTTP.ListenAndServe()
	})
	group.Go(func() error { return a.Scheduler.Run(ctx) })
	group.Go(func() error {
		<-ctx.Done()
		return a.shutdown()
	})
	return normalizeServerClosed(group.Wait())
}
