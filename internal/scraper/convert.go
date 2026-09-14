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
		LogoUrl:        LogoURLForSource(source.SourceId, source.LogoURL),
	}
}

// LogoURLForSource returns the URL the LogoScheduler should download
// for a team. The source-agnostic Team.LogoURL is empty when the
// upstream payload does not include the field — FotMob's 2026
// /api/data/matches payload is one such case. As a fallback, derive
// the public SofaScore CDN URL for the team ID so the LogoScheduler
// has something to download.
//
// The CDN URL pattern (https://img.sofascore.com/api/v1/team/<id>/image)
// is the same one the SofaScore web app and the img.sofascore.com CDN
// serve; it returns the team badge PNG with no further host allow-list
// restrictions. Verified by hand against the live CDN on 2026-09-13
// (returns HTTP 200 with Referer https://img.sofascore.com/).
//
// Earlier we pointed the fallback at images.fotmob.com/image_resources/
// logo/teamlogo_<id>.png; that URL pattern is rejected by the
// CloudFront distribution in front of the bucket (403 AccessDenied
// for every Referer we tried). SofaScore's CDN accepts the standard
// library TLS fingerprint and only checks Referer, which is what the
// LogoScheduler already sends.
//
// Source IDs in the shared `teams` table are prefixed per source to
// avoid collisions: 7_000_000_000 for scores365 (see
// internal/scraper/scores365/source.go::TeamIDPrefix), 2_000_000_000
// for the now-removed TheSportsDB. SofaScore's CDN is keyed by the
// upstream's natural ID, so we strip the prefix before building the
// URL.
//
// If LogoURL is already set (e.g. for a future source that ships a
// full image URL), it is returned verbatim — the source wins.
func LogoURLForSource(sourceID int64, logoURL string) string {
	if logoURL != "" {
		return logoURL
	}
	if sourceID == 0 {
		return ""
	}
	natural := sourceID
	switch {
	case natural >= 7_000_000_000:
		natural -= 7_000_000_000
	case natural >= 2_000_000_000:
		natural -= 2_000_000_000
	}
	return "https://img.sofascore.com/api/v1/team/" + strconv.FormatInt(natural, 10) + "/image"
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
		LeagueId:        uint(parseLeagueID(source.League.SourceLeagueId)),
	}
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
