package scheduler

import (
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
	"github.com/robfig/cron/v3"
)

func startStats() {
	c := cron.New()
	_, _ = c.AddFunc("1 0 * * *", func() {
		_ = repository.GenerateDailyEventStats()
	})

	_, _ = c.AddFunc("10 0 1 * *", func() {
		_ = repository.GenerateMonthlyEventStats()
	})

	c.Start()
}
