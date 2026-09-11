package scraper

import (
	"sort"
	"strconv"
	"strings"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func ToTeam(source Team) events.Team {
	return events.Team{
		TeamId:         source.SourceId,
		Name:           source.Name,
		PrimaryColor:   source.PrimaryColor,
		SecondaryColor: source.SecondaryColor,
		TextColor:      source.TextColor,
		LogoUrl:        source.LogoURL,
	}
}

func ToTournament(source LeagueRef) tournaments.Tournament {
	slug := strings.ToLower(strings.ReplaceAll(source.Name, " ", "-"))
	return tournaments.Tournament{
		Model:  gorm.Model{ID: parseLeagueID(source.SourceLeagueId)},
		Name:   source.Name,
		Slug:   slug,
		Region: source.Country,
	}
}

func ToEvent(source Match, sport string) events.Event {
	homeTeam := ToTeam(source.HomeTeam)
	awayTeam := ToTeam(source.AwayTeam)
	tournament := ToTournament(source.League)

	startTs := source.StartTimestamp.UnixMilli()
	if startTs < 0 || startTs > 1<<62/1000 {
		startTs = 0
	}

	return events.Event{
		ExternalMatchId: source.SourceMatchId,
		Source:          source.Source,
		Sport:           sport,
		Slug:            source.Slug,
		StartTimestamp:  startTs,
		StatusType:      source.Status.Type,
		HomeTeamId:      homeTeam.TeamId,
		AwayTeamId:      awayTeam.TeamId,
		HomeTeamModel:   &homeTeam,
		AwayTeamModel:   &awayTeam,
		League:          &tournament,
		LeagueId:        uint(parseLeagueID(source.League.SourceLeagueId)),
	}
}

// parseLeagueID convierte el ID string de FotMob a uint. Si falla, retorna 0
// (la fila no se podrá relacionar con un league_id válido pero igual se persiste).
func parseLeagueID(s string) uint {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

func ToScrapeBatch(matches []Match, sport string) events.ScrapeBatch {
	teamMap := make(map[int64]events.Team)
	tournamentMap := make(map[uint]tournaments.Tournament)
	eventMap := make(map[string]events.Event)

	for _, m := range matches {
		home := ToTeam(m.HomeTeam)
		away := ToTeam(m.AwayTeam)
		tournament := ToTournament(m.League)
		event := ToEvent(m, sport)

		teamMap[home.TeamId] = home
		teamMap[away.TeamId] = away
		tournamentMap[tournament.ID] = tournament
		eventMap[event.ExternalMatchId] = event
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
	sort.Slice(evts, func(i, j int) bool { return evts[i].ExternalMatchId < evts[j].ExternalMatchId })

	return events.ScrapeBatch{Teams: teams, Tournaments: tours, Events: evts}
}
