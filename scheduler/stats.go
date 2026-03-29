package scheduler

import "github.com/jeriveromartinez/sofascore-scrapper/repository"

func startStats() {
	_ = repository.GenerateDailyEventStats()
	_ = repository.GenerateMonthlyEventStats()
}
