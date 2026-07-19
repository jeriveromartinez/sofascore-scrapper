package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/app"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 && os.Args[1] == "bootstrap-invitation" {
		runBootstrapInvitation(cfg)
		return
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := application.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func runBootstrapInvitation(cfg config.Config) {
	db, _, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}

	var count int64
	if err := db.Model(&struct{ gorm.Model }{}).Table("users").Count(&count).Error; err != nil {
		log.Fatal(err)
	}

	if count > 0 {
		fmt.Fprintln(os.Stderr, "users table is not empty, refusing to bootstrap invitation")
		os.Exit(1)
	}

	client, err := redisplatform.New(context.Background(), cfg.Redis)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	store := auth.NewInvitationStore(client)
	token, _, err := store.Create(context.Background(), auth.DefaultInvitationTTL)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(token)
}
