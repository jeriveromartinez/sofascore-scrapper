package repository

import (
	"strings"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"gorm.io/gorm"
)

func eventsRepo() (*events.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return events.NewRepository(db), nil
}

func SaveSofaScoreEvent(apiEvents []*models.APIEvent, sport string) {
	repo, err := eventsRepo()
	if err != nil {
		return
	}

	evs := make([]events.Event, 0, len(apiEvents))
	for _, apiEvent := range apiEvents {
		e := apiEvent.ToSofaScoreEvent()
		homeTeam := apiEvent.HomeTeam.ToSofaScoreTeam()
		awayTeam := apiEvent.AwayTeam.ToSofaScoreTeam()

		e.HomeTeamModel = &homeTeam
		e.AwayTeamModel = &awayTeam

		tournamentSlug := apiEvent.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(apiEvent.Tournament.UniqueTournament.Category.Slug)
		e.League = &tournaments.Tournament{
			Model:  gorm.Model{ID: uint(apiEvent.Tournament.UniqueTournament.ID)},
			Name:   apiEvent.Tournament.UniqueTournament.Name,
			Slug:   tournamentSlug,
			Region: apiEvent.Tournament.UniqueTournament.Category.Name,
		}

		evs = append(evs, e)
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
