package scheduler

import (
	"context"
	"log"
	"sync"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

func startStats(ctx context.Context, db interface{}, aggRepo interface{}, wg *sync.WaitGroup) {
	gormDB, ok := db.(*gorm.DB)
	if !ok || gormDB == nil {
		log.Printf("scheduler: no DB available for stats")
		return
	}

	agg, ok := aggRepo.(*reporting.AggregationRepository)
	if !ok || agg == nil {
		return
	}

	c := cron.New()

	_, err := c.AddFunc("1 0 * * *", func() {
		if err := agg.GenerateDaily(); err != nil {
			log.Printf("failed to generate daily event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule daily stats cron job: %v", err)
	}

	_, err = c.AddFunc("10 0 1 * *", func() {
		if err := agg.GenerateMonthly(); err != nil {
			log.Printf("failed to generate monthly event stats: %v", err)
		}
	})
	if err != nil {
		log.Printf("failed to schedule monthly stats cron job: %v", err)
	}

	c.Start()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.Stop()
	}()
}
