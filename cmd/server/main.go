package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/app"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/seeder"
	"gorm.io/gorm"
)

func main() {
	logger := observability.NewLogger(slog.LevelInfo, os.Stdout)
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if len(os.Args) > 1 && os.Args[1] == "bootstrap-invitation" {
		runBootstrapInvitation(cfg)
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(cfg)
		return
	}

	application, err := app.New(cfg)
	if err != nil {
		logger.Error("failed to create app", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := application.Run(ctx); err != nil {
		logger.Error("application error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func runBootstrapInvitation(cfg config.Config) {
	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := database.AutoMigrateAll(db); err != nil {
		slog.Error("failed to automigrate", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		slog.Error("failed to seed default admin", slog.String("error", err.Error()))
		os.Exit(1)
	}

	var count int64
	if err := db.Model(&struct{ gorm.Model }{}).Table("users").Count(&count).Error; err != nil {
		slog.Error("failed to count users", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if count > 0 {
		fmt.Fprintln(os.Stderr, "users table is not empty, refusing to bootstrap invitation")
		os.Exit(1)
	}

	client, err := redisplatform.New(context.Background(), cfg.Redis)
	if err != nil {
		slog.Error("failed to connect to redis", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer client.Close()

	store := auth.NewInvitationStore(client)
	token, _, err := store.Create(context.Background(), auth.DefaultInvitationTTL)
	if err != nil {
		slog.Error("failed to create invitation", slog.String("error", err.Error()))
		os.Exit(1)
	}

	fmt.Println(token)
}

func runMigrate(cfg config.Config) {
	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := database.AutoMigrateAll(db); err != nil {
		slog.Error("failed to automigrate", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		slog.Error("failed to seed default admin", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("migrate: schema synced and default admin seeded")
}
