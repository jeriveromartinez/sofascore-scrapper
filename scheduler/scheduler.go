package scheduler

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/httpcli"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func scrapeDate(sport string, date time.Time) {
	body := httpcli.LoadData(sport, date)
	var list models.EventsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		log.Printf("scheduler: error parsing JSON for %s on %s: %v", sport, date.Format("2006-01-02"), err)
		return
	}
	repository.SaveSofaScoreEvent(list.Events, sport)
	log.Printf("scheduler: scraped %d events for %s on %s", len(list.Events), sport, date.Format("2006-01-02"))
}

func scrapeNext7Days() {
	now := time.Now()
	for i := 1; i <= 7; i++ {
		scrapeDate(httpcli.FOOTBALL, now.Add(time.Duration(i)*24*time.Hour))
	}
}

func Start() {
	// Every minute: scrape today
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			scrapeDate(httpcli.FOOTBALL, time.Now())
		}
	}()

	// Twice daily at 06:00 and 18:00 UTC: scrape next 7 days
	go func() {
		for {
			now := time.Now().UTC()
			h := now.Hour()
			var next time.Time
			if h < 6 {
				next = time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
			} else if h < 18 {
				next = time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, time.UTC)
			} else {
				next = time.Date(now.Year(), now.Month(), now.Day()+1, 6, 0, 0, 0, time.UTC)
			}
			time.Sleep(time.Until(next))
			scrapeNext7Days()
		}
	}()
}
