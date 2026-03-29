package scheduler

import (
	"log"

	"github.com/jeriveromartinez/sofascore-scrapper/repository"
	"github.com/robfig/cron/v3"
)

func startStats() {
	c := cron.New()

	_, err := c.AddFunc("1 0 * * *", func() {
		if err := repository.GenerateDailyEventStats(); err != nil {
			log.Printf("failed to generate daily event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule daily stats cron job: %v", err)
	}

	_, err = c.AddFunc("10 0 1 * *", func() {
		if err := repository.GenerateMonthlyEventStats(); err != nil {
			log.Printf("failed to generate monthly event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule monthly stats cron job: %v", err)
	}

	c.Start()
}
