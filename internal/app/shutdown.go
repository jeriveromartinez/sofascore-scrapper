package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
)

func (a *App) shutdown() error {
	a.logger.Info("shutting down application")

	a.ready.Store(false)
	if a.logoScheduler != nil {
		a.logoScheduler.Stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.HTTP.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("HTTP shutdown error", slog.String("error", err.Error()))
	}

	if a.Pprof != nil {
		pprofCtx, pprofCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pprofCancel()
		if err := observability.PprofShutdown(pprofCtx, a.Pprof); err != nil {
			a.logger.Error("pprof shutdown error", slog.String("error", err.Error()))
		}
	}

	a.Scheduler.Shutdown()
	if a.logoScheduler != nil {
		a.logoScheduler.Shutdown(shutdownCtx)
	}

	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			a.logger.Error("Redis close error", slog.String("error", err.Error()))
		}
	}

	if a.SQL != nil {
		if err := a.SQL.Close(); err != nil {
			a.logger.Error("SQL close error", slog.String("error", err.Error()))
		}
	}

	a.logger.Info("shutdown complete")
	return nil
}

func normalizeServerClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func buildCleanupJobFromApp(a *App) *apk.CleanupJob {
	store := apk.NewUploadStateStore(a.Redis)
	chunkStore := apk.NewChunkStore(a.storagePath)
	return apk.NewCleanupJob(store, chunkStore)
}
