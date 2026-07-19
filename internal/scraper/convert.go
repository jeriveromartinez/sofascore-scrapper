package scraper

import (
	"fmt"
	"sort"
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

func ToTournament(source APIEvent) tournaments.Tournament {
	slug := source.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(source.Tournament.UniqueTournament.Category.Slug)
	return tournaments.Tournament{
		Model:  gorm.Model{ID: uint(source.Tournament.UniqueTournament.ID)},
		Name:   source.Tournament.UniqueTournament.Name,
		Slug:   slug,
		Region: source.Tournament.UniqueTournament.Category.Name,
	}
}

func ToEvent(source APIEvent, sport string) events.Event {
	homeTeam := ToTeam(source.HomeTeam)
	awayTeam := ToTeam(source.AwayTeam)
	tournamentSlug := source.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(source.Tournament.UniqueTournament.Category.Slug)

	startTs := int64(0)
	if source.StartTimestamp > 0 && source.StartTimestamp < 1<<62/1000 {
		startTs = source.StartTimestamp * 1000
	}
	currentPeriodTs := int64(0)
	if source.Time.CurrentPeriodStartTimestamp > 0 && source.Time.CurrentPeriodStartTimestamp < 1<<62/1000 {
		currentPeriodTs = source.Time.CurrentPeriodStartTimestamp * 1000
	}

	return events.Event{
		SofaScoreEventId:            source.ID,
		HomeScore:                   source.HomeScore.Current,
		HomeTeamId:                  source.HomeTeam.ID,
		AwayScore:                   source.AwayScore.Current,
		AwayTeamId:                  source.AwayTeam.ID,
		StartTimestamp:              startTs,
		CurrentPeriodStartTimestamp: currentPeriodTs,
		Slug:                        source.Slug,
		LeagueId:                    uint(source.Tournament.UniqueTournament.ID),
		Sport:                       sport,
		StatusType:                  source.Status.Type,
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

func ToScrapeBatch(apiEvents []*APIEvent, sport string) events.ScrapeBatch {
	teamMap := make(map[int64]events.Team)
	tournamentMap := make(map[uint]tournaments.Tournament)
	eventMap := make(map[int64]events.Event)

	for _, apiEvent := range apiEvents {
		homeTeam := ToTeam(apiEvent.HomeTeam)
		awayTeam := ToTeam(apiEvent.AwayTeam)
		tournament := ToTournament(*apiEvent)
		event := ToEvent(*apiEvent, sport)

		teamMap[homeTeam.TeamId] = homeTeam
		teamMap[awayTeam.TeamId] = awayTeam
		tournamentMap[tournament.ID] = tournament
		eventMap[event.SofaScoreEventId] = event
	}

	teams := make([]events.Team, 0, len(teamMap))
	for _, t := range teamMap {
		teams = append(teams, t)
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].TeamId < teams[j].TeamId })

	tours := make([]tournaments.Tournament, 0, len(tournamentMap))
	for _, t := range tournamentMap {
		tours = append(tours, t)
	}
	sort.Slice(tours, func(i, j int) bool { return tours[i].ID < tours[j].ID })

	evts := make([]events.Event, 0, len(eventMap))
	for _, e := range eventMap {
		e.HomeTeamModel = nil
		e.AwayTeamModel = nil
		e.League = nil
		evts = append(evts, e)
	}
	sort.Slice(evts, func(i, j int) bool { return evts[i].SofaScoreEventId < evts[j].SofaScoreEventId })

	return events.ScrapeBatch{
		Teams:       teams,
		Tournaments: tours,
		Events:      evts,
	}
}
