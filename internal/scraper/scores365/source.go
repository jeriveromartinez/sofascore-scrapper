package scores365

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
)

// TeamIDPrefix namespaces 365scores competitor IDs into the shared
// `teams` table. The prefix 7_000_000_000 sits well above the
// FotMob range (their IDs go up to ~2M) and the (now-removed)
// TheSportsDB prefix of 2_000_000_000, so any cross-source
// collision is impossible.
const TeamIDPrefix int64 = 7_000_000_000

// LeagueIDPrefix is re-exported from the upstream scores365 package
// so the discovery code can apply the same namespace prefix when
// seeding scraper_leagues from the competitions API (see
// internal/scraper/catalog/discovery.go::toLeagueRef). Keeping the
// constant owned by the upstream package avoids a directional
// dependency from the catalog package back into this source
// implementation (which would create an import cycle).
const LeagueIDPrefix = scores365.LeagueIDPrefix

// Source implements scraper.Source on top of the 365scores web API's
// /web/games/allscores/ day-wide endpoint. It also satisfies
// scraper.DayMatcher so the dispatcher can route per-sport and
// auto-create leagues.
type Source struct {
	client *scores365.Client
	logger *slog.Logger
}

// NewSource wraps a scores365.Client with the Source interface.
func NewSource(client *scores365.Client) *Source {
	return &Source{client: client, logger: slog.Default()}
}

func (s *Source) Name() string { return "scores365" }

// DayMatches returns every match 365scores publishes for the given
// calendar day. The HTTP layer already caches per request URL for
// 60s, so calling DayMatches multiple times within the same scrape
// loop is cheap.
//
// Unlike the legacy /data/games feed this works for arbitrary past
// and future dates: /web/games/allscores/ takes an explicit
// startDate/endDate pair, so ScrapeNext7Days gets real data for all
// seven iterations instead of only for today.
func (s *Source) DayMatches(ctx context.Context, date time.Time) ([]scraper.Match, error) {
	feed, err := s.client.FetchGames(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("scores365: DayMatches fetch %s: %w", date.Format("2006-01-02"), err)
	}
	countries := indexCountries(feed.Countries)
	competitions := indexCompetitions(feed.Competitions)
	out := make([]scraper.Match, 0, len(feed.Games))
	for _, raw := range feed.Games {
		match, ok := s.gameToMatch(raw, competitions, countries)
		if !ok {
			continue
		}
		out = append(out, match)
	}
	return out, nil
}

// ScheduledEvents returns the subset of DayMatches filtered by
// league. Used as a fallback by the dispatcher when the bulk
// day-wide path is not appropriate.
func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	all, err := s.DayMatches(ctx, date)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0, 8)
	for _, m := range all {
		if m.League.SourceLeagueId == league.SourceLeagueId {
			out = append(out, m)
		}
	}
	return out, nil
}

// SearchLeagues is intentionally not implemented: scores365 leagues
// are enrolled by the boot-time catalog discovery sweep (which reads
// /web/competitions/) and managed from the admin catalog afterwards.
func (s *Source) SearchLeagues(_ context.Context, _ string) ([]scraper.LeagueSearchResult, error) {
	return nil, errors.New("scores365: SearchLeagues is not supported; add leagues via the catalog admin form")
}

func indexCountries(countries []scores365.Country) map[int]scores365.Country {
	out := make(map[int]scores365.Country, len(countries))
	for _, c := range countries {
		out[c.ID] = c
	}
	return out
}

func indexCompetitions(competitions []scores365.Competition) map[int]scores365.Competition {
	out := make(map[int]scores365.Competition, len(competitions))
	for _, c := range competitions {
		out[c.ID] = c
	}
	return out
}

// gameToMatch converts a wire Game into the scraper.Match shape.
// Returns (zero, false) when the game lacks essential fields.
func (s *Source) gameToMatch(
	raw scores365.Game,
	competitions map[int]scores365.Competition,
	countries map[int]scores365.Country,
) (scraper.Match, bool) {
	if raw.ID == 0 || raw.CompetitionID == 0 {
		return scraper.Match{}, false
	}
	if raw.HomeCompetitor.ID == 0 || raw.AwayCompetitor.ID == 0 {
		return scraper.Match{}, false
	}

	startTS, ok := parseStartTime(raw.StartTime)
	if !ok {
		s.logger.Warn("scores365: skip game unparseable startTime",
			slog.Int("game_id", raw.ID),
			slog.String("start_time", raw.StartTime))
		return scraper.Match{}, false
	}

	kind := classify(raw)
	homeScore, awayScore := scorePair(raw, kind)
	leagueName, leagueCountry := lookupLeague(raw, competitions, countries)

	return scraper.Match{
		Source:         s.Name(),
		SourceMatchId:  "scores365-" + strconv.Itoa(raw.ID),
		StartTimestamp: startTS,
		HomeScore:      homeScore,
		AwayScore:      awayScore,
		Status:         statusFlags(raw, kind),
		HomeTeam:       s.teamFromCompetitor(raw.HomeCompetitor),
		AwayTeam:       s.teamFromCompetitor(raw.AwayCompetitor),
		League: scraper.LeagueRef{
			Source:         s.Name(),
			SourceLeagueId: strconv.FormatInt(LeagueIDPrefix+int64(raw.CompetitionID), 10),
			Name:           leagueName,
			Sport:          sportSlugFromID(raw.SportID),
			Country:        leagueCountry,
		},
	}, true
}

// lookupLeague resolves the human-readable league name and country
// for a game. The name prefers the competition dictionary entry, then
// the per-game display name (which carries the phase/zone suffix,
// e.g. "Primera C Metropolitana - Zona B"). The dispatch path does
// not fill these from the catalog, so they must be set here for the
// persisted tournament row to be useful.
func lookupLeague(
	raw scores365.Game,
	competitions map[int]scores365.Competition,
	countries map[int]scores365.Country,
) (string, string) {
	comp, ok := competitions[raw.CompetitionID]
	if !ok {
		return raw.CompetitionDisplayName, ""
	}
	name := comp.Name
	if name == "" {
		name = raw.CompetitionDisplayName
	}
	if country, ok := countries[comp.CountryID]; ok {
		return name, country.Name
	}
	return name, ""
}

// teamFromCompetitor builds a scraper.Team from a wire Competitor.
// The ID is prefixed with TeamIDPrefix so the row lands in the shared
// `teams` table without colliding with any FotMob-sourced row.
//
// LogoURL is set from the 365scores image CDN (keyed by the team's
// natural ID plus its imageVersion cache-buster), so the
// LogoScheduler downloads the badge straight from the source that
// published the fixture instead of guessing at a third-party CDN.
func (s *Source) teamFromCompetitor(t scores365.Competitor) scraper.Team {
	return scraper.Team{
		SourceId:       TeamIDPrefix + int64(t.ID),
		Name:           t.Name,
		LogoURL:        s.client.CompetitorImageURL(t.ID, t.ImageVersion),
		PrimaryColor:   t.Color,
		SecondaryColor: t.AwayColor,
	}
}

// scorePair returns the (home, away) score for a game. 365scores
// reports -1 on both sides for a fixture that has not produced a
// score yet; those become 0 so the persisted row does not carry a
// negative sentinel into the API.
func scorePair(raw scores365.Game, kind statusKind) (int, int) {
	if kind == statusScheduled {
		return 0, 0
	}
	return clampScore(raw.HomeCompetitor.Score), clampScore(raw.AwayCompetitor.Score)
}

func clampScore(v float64) int {
	if v <= scores365.ScoreNone {
		return 0
	}
	return int(v)
}

// parseStartTime converts the wire startTime into UTC.
//
// The web API returns RFC3339 with the offset of the timezoneName
// the request asked for (e.g. "2026-09-14T12:00:00-06:00"), so the
// instant is unambiguous; we normalise to UTC because that is what
// events.Event stores.
func parseStartTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// statusFlags returns the MatchStatus for a wire Game.
//
// The events repository queries against the (Started, Finished,
// Cancelled) bool triple, not against the source-specific label, so
// the statusGroup/statusText classification is translated into that
// triple here. Code and Description keep the upstream's own
// (localized) labels for downstream consumers that want something
// human-readable.
func statusFlags(raw scores365.Game, kind statusKind) scraper.MatchStatus {
	st := scraper.MatchStatus{
		Code:        raw.StatusGroup,
		Description: raw.StatusText,
	}
	switch kind {
	case statusCancelled:
		st.Cancelled = true
		st.Type = "CANC"
	case statusLive:
		st.Started = true
		st.Type = "LIVE"
	case statusFinished:
		st.Started = true
		st.Finished = true
		st.Type = "FT"
	default:
		st.Type = "NS"
	}
	return st
}
