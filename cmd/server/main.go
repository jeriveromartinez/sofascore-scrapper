package main

import (
	"context"
	"log"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/app"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := application.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
