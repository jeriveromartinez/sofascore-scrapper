package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scheduler"
	"gorm.io/gorm"
)

type App struct {
	HTTP      *http.Server
	Scheduler *scheduler.Scheduler
	DB        *gorm.DB
	SQL       *sql.DB
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

	router := NewRouter(db, redisClient, cfg, tokens)
	httpServer := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	sched := scheduler.New()

	return &App{
		HTTP:      httpServer,
		Scheduler: sched,
		DB:        db,
		SQL:       sqlDB,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	scrapeSvc, aggRepo := buildSchedulerDeps(a.DB)
	a.Scheduler.Start(ctx, a.DB, scrapeSvc, aggRepo)

	go func() {
		log.Printf("API server listening on %s", a.HTTP.Addr)
		if err := a.HTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		log.Printf("received signal %v, shutting down", sig)
	case <-ctx.Done():
		log.Println("context cancelled, shutting down")
	}

	a.Scheduler.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.HTTP.Shutdown(shutdownCtx); err != nil {
		return err
	}

	if err := a.SQL.Close(); err != nil {
		return err
	}

	return nil
}
