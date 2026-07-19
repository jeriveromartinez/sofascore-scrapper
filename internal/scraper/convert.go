package scraper

import (
	"fmt"
	"strings"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func ToTeam(source TeamApi) events.Team {
	return events.Team{
		TeamId:         source.ID,
		Name:           source.Name,
		PrimaryColor:   source.Colors.Primary,
		SecondaryColor: source.Colors.Secondary,
		TextColor:      source.Colors.Text,
		LogoUrl:        "https://img.sofascore.com/api/v1/team/" + fmt.Sprint(source.ID) + "/image",
	}
}

func ToEvent(source APIEvent, sport string) events.Event {
	homeTeam := ToTeam(source.HomeTeam)
	awayTeam := ToTeam(source.AwayTeam)
	tournamentSlug := source.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(source.Tournament.UniqueTournament.Category.Slug)

	return events.Event{
		SofaScoreEventId:            source.ID,
		HomeScore:                   source.HomeScore.Current,
		HomeTeamId:                  source.HomeTeam.ID,
		AwayScore:                   source.AwayScore.Current,
		AwayTeamId:                  source.AwayTeam.ID,
		StartTimestamp:              source.StartTimestamp,
		CurrentPeriodStartTimestamp: source.Time.CurrentPeriodStartTimestamp,
		Slug:                        source.Slug,
		LeagueId:                    uint(source.Tournament.UniqueTournament.ID),
		Sport:                       sport,
		HomeTeamModel:               &homeTeam,
		AwayTeamModel:               &awayTeam,
		League: &tournaments.Tournament{
			Model:  gorm.Model{ID: uint(source.Tournament.UniqueTournament.ID)},
			Name:   source.Tournament.UniqueTournament.Name,
			Slug:   tournamentSlug,
			Region: source.Tournament.UniqueTournament.Category.Name,
		},
	}
}
