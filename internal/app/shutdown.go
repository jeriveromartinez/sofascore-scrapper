package app

import (
	"context"
	"log"
	"net/http"
	"time"
)

func (a *App) shutdown() error {
	log.Println("shutting down application")

	a.ready.Store(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.HTTP.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	a.Scheduler.Shutdown()

	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			log.Printf("Redis close error: %v", err)
		}
	}

	if a.SQL != nil {
		if err := a.SQL.Close(); err != nil {
			log.Printf("SQL close error: %v", err)
		}
	}

	log.Println("shutdown complete")
	return nil
}

func normalizeServerClosed(err error) error {
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
