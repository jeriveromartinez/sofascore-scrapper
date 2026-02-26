package main

import (
	"log"
	"os"

	"github.com/jeriveromartinez/sofascore-scrapper/api"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/scheduler"
)

func main() {
	models.Migrate()
	scheduler.Start()
	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Println("Starting API server and scheduler...")
	api.Start(addr)
}
