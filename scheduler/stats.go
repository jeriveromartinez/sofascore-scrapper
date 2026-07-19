package scheduler

import (
	"log"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/robfig/cron/v3"
)

func startStats() {
	c := cron.New()

	_, err := c.AddFunc("1 0 * * *", func() {
		db, dbErr := database.GetDB()
		if dbErr != nil {
			log.Printf("failed to get DB for daily stats: %v", dbErr)
			return
		}
		if err := reporting.NewAggregationRepository(db).GenerateDaily(); err != nil {
			log.Printf("failed to generate daily event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule daily stats cron job: %v", err)
	}

	_, err = c.AddFunc("10 0 1 * *", func() {
		db, dbErr := database.GetDB()
		if dbErr != nil {
			log.Printf("failed to get DB for monthly stats: %v", dbErr)
			return
		}
		if err := reporting.NewAggregationRepository(db).GenerateMonthly(); err != nil {
			log.Printf("failed to generate monthly event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule monthly stats cron job: %v", err)
	}

	c.Start()
}
