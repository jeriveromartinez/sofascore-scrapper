package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func eventsRepo() (*events.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return events.NewRepository(db), nil
}

func SaveSofaScoreEvent(apiEvents []*scraper.APIEvent, sport string) {
	repo, err := eventsRepo()
	if err != nil {
		return
	}

	evs := make([]events.Event, 0, len(apiEvents))
	for _, apiEvent := range apiEvents {
		evs = append(evs, scraper.ToEvent(*apiEvent, sport))
	}

	repo.Save(evs, sport)
}

func GetCurrentAndUpcomingEvents(devID uint, limit int) ([]events.Event, error) {
	repo, err := eventsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetCurrentAndUpcoming(devID, limit)
}
