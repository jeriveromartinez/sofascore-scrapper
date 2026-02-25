package main

import (
	"log"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/scraper"
)

func main() {
	// Connect to the MariaDB database.
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}

	log.Println("Starting Sofascore scraper...")

	// Scrape sports events from Sofascore.
	events, err := scraper.Scrape()
	if err != nil {
		log.Fatalf("Scraping failed: %v", err)
	}

	if len(events) == 0 {
		log.Println("No events found.")
		return
	}

	// Save all scraped events to the database.
	result := db.Create(&events)
	if result.Error != nil {
		log.Fatalf("Error saving events to database: %v", result.Error)
	}

	log.Printf("Successfully saved %d events to the database.", result.RowsAffected)
}
