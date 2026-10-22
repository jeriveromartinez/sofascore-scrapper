package scraper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func ToTeam(source Team) events.Team {
	return events.Team{
		TeamId:         source.SourceId,
		Name:           source.Name,
		PrimaryColor:   source.PrimaryColor,
		SecondaryColor: source.SecondaryColor,
		TextColor:      source.TextColor,
		LogoUrl:        LogoURLForSource(source.SourceId, source.LogoURL),
	}
}

// LogoURLForSource returns the URL the LogoScheduler should download
// for a team.
//
// If LogoURL is already set the source wins and it is returned
// verbatim — the 365scores source fills it in from the competitor's
// own badge on imagecache.365scores.com, so that path is the common
// one. The source-agnostic Team.LogoURL is empty when the upstream
// payload does not include the field (FotMob's /api/data/matches
// payload is one such case); those fall back to
// events.TeamLogoSourceURL, which derives the CDN URL from the team
// ID's source namespace.
//
// events owns that mapping so the scheduler's fallback chain and this
// conversion can never disagree about which CDN a team belongs to.
func LogoURLForSource(sourceID int64, logoURL string) string {
	if logoURL != "" {
		return logoURL
	}
	if sourceID == 0 {
		return ""
	}
	return events.TeamLogoSourceURL(sourceID)
}

// FotMobTournamentIDBase is the base of the reserved id range that
// holds the FotMob league ids in the tournaments table. The table is
// shared with the admin UI, whose rows get auto-increment ids
// starting at 1, so the upstream league ids need a range of their own
// or an admin-created tournament overwrites the scraped league that
// carries the same number.
const FotMobTournamentIDBase uint64 = tournaments.FotMobTournamentIDBase

// TournamentID maps an upstream league onto the primary key of the
// tournaments table.
//
// The 365scores league ids already carry the upstream prefix
// (scores365.LeagueIDPrefix) and are used as they are; the FotMob ids
// are shifted into the range reserved for them. Both ranges sit far
// above the auto-increment ids the admin UI hands out.
func TournamentID(league LeagueRef) (uint, error) {
	raw, err := parseLeagueID(league.SourceLeagueId)
	if err != nil {
		return 0, err
	}
	if league.Source == "scores365" {
		return uint(raw), nil
	}
	return uint(FotMobTournamentIDBase + raw), nil
}

// ToTournament maps a league onto the tournaments row the events
// point at. A malformed league id is an error: it used to become the
// primary key 0, which silently collected every league whose id the
// upstream failed to publish.
func ToTournament(source LeagueRef) (tournaments.Tournament, error) {
	id, err := TournamentID(source)
	if err != nil {
		return tournaments.Tournament{}, err
	}
	slug := strings.ToLower(strings.ReplaceAll(source.Name, " ", "-"))
	return tournaments.Tournament{
		Model:  gorm.Model{ID: id},
		Name:   source.Name,
		Slug:   slug,
		Region: source.Country,
	}, nil
}

func ToEvent(source Match, sport string) (events.Event, error) {
	homeTeam := ToTeam(source.HomeTeam)
	awayTeam := ToTeam(source.AwayTeam)
	tournament, err := ToTournament(source.League)
	if err != nil {
		return events.Event{}, err
	}

	startTs := source.StartTimestamp.UnixMilli()
	if startTs < 0 || startTs > 1<<62/1000 {
		startTs = 0
	}

	// PR #124: scores come straight from the source's per-team
	// int fields (FotMob exposes home.score / away.score on the
	// /api/data/matches payload). Status.ScoreStr is kept around
	// for sources that only expose a string ("2-1"), but the int
	// halves are authoritative.
	homeScore := source.HomeScore
	awayScore := source.AwayScore

	return events.Event{
		ExternalMatchId: source.SourceMatchId,
		Source:          source.Source,
		Sport:           sport,
		Slug:            source.Slug,
		StartTimestamp:  startTs,
		StatusType:      normalizeStatus(source.Status),
		HomeTeamId:      homeTeam.TeamId,
		AwayTeamId:      awayTeam.TeamId,
		HomeTeamModel:   &homeTeam,
		AwayTeamModel:   &awayTeam,
		HomeScore:       homeScore,
		AwayScore:       awayScore,
		League:          &tournament,
		LeagueId:        tournament.ID,
	}, nil
}

// parseScoreStr previously parsed "2-1" strings out of
// MatchStatus.ScoreStr. PR #124 moved the source of truth to
// Match.HomeScore and Match.AwayScore (FotMob /api/data/matches
// exposes per-team int scores directly), so this helper is no
// longer called. It is retained here as a build target so that the
// parseScoreStr-related symbols (strings, strconv) are not
// dropped from the import list by accident — the body is dead
// code guarded by an unconditional panic if anything ever calls
// it, which makes the regression loud.
// Deprecated: use Match.HomeScore / Match.AwayScore directly.
func parseScoreStr(string) (int, int) {
	panic("scraper.parseScoreStr is deprecated; use Match.HomeScore/Match.AwayScore (PR #124)")
}

// normalizeStatus rewrites the FotMob (started, finished, cancelled)
// bool triple into the values the events repository queries against.
// The repository only selects events with status_type in
// {notstarted, inprogress}, so any value outside the mapping below
// (or an unrecognized status from a future source) becomes "" so the
// row simply does not match a WHERE clause rather than silently
// matching one.
//
// Mapping (fix B3, PR #122):
//
//	cancelled=true                             -> "cancelled"
//	started=true && finished=true              -> "finished"
//	started=true && finished=false             -> "inprogress"
//	started=false && finished=false            -> "notstarted"
//	anything else                              -> ""
func normalizeStatus(s MatchStatus) string {
	if s.Cancelled {
		return "cancelled"
	}
	switch {
	case s.Started && s.Finished:
		return "finished"
	case s.Started && !s.Finished:
		return "inprogress"
	case !s.Started && !s.Finished:
		return "notstarted"
	default:
		return ""
	}
}

// parseLeagueID parses the string form of an upstream league id. A
// malformed id is rejected: it used to become 0, which the tournaments
// table then stored as a primary key of its own.
func parseLeagueID(s string) (uint64, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("scraper: invalid league id %q: %w", s, err)
	}
	if n == 0 {
		return 0, fmt.Errorf("scraper: league id must not be zero")
	}
	return n, nil
}

func ToScrapeBatch(matches []Match, sport string) (events.ScrapeBatch, error) {
	teamMap := make(map[int64]events.Team)
	tournamentMap := make(map[uint]tournaments.Tournament)
	eventMap := make(map[string]events.Event)

	for _, m := range matches {
		home := ToTeam(m.HomeTeam)
		away := ToTeam(m.AwayTeam)
		tournament, err := ToTournament(m.League)
		if err != nil {
			return events.ScrapeBatch{}, err
		}
		event, err := ToEvent(m, sport)
		if err != nil {
			return events.ScrapeBatch{}, err
		}

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

	return events.ScrapeBatch{Teams: teams, Tournaments: tours, Events: evts}, nil
}
