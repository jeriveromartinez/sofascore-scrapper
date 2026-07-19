package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
)

func (a *App) shutdown() error {
	a.logger.Info("shutting down application")

	a.ready.Store(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.HTTP.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("HTTP shutdown error", slog.String("error", err.Error()))
	}

	a.Scheduler.Shutdown()

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
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func buildCleanupJobFromApp(a *App) *apk.CleanupJob {
	store := apk.NewUploadStateStore(a.Redis)
	chunkStore := apk.NewChunkStore(a.storagePath)
	return apk.NewCleanupJob(store, chunkStore)
}
