package scores365

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scores365"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

// TeamIDPrefix namespaces 365scores team IDs into the shared
// `teams` table. The prefix 7_000_000_000 sits well above the
// FotMob range (their IDs go up to ~2M) and the (now-removed)
// TheSportsDB prefix of 2_000_000_000, so any cross-source
// collision is impossible.
const TeamIDPrefix int64 = 7_000_000_000

// LeagueIDPrefix is re-exported from the upstream scores365 package
// so the discovery code can apply the same namespace prefix when
// seeding scraper_leagues from the sitemap feed (see
// internal/scraper/catalog/discovery.go::toLeagueRef). Keeping the
// constant owned by the upstream package avoids a directional
// dependency from the catalog package back into this source
// implementation (which would create an import cycle).
const LeagueIDPrefix = scores365.LeagueIDPrefix

// Source implements scraper.Source on top of 365scores' /data/games
// day-wide endpoint. It also satisfies scraper.DayMatcher so the
// dispatcher can route per-sport and auto-create leagues.
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
// UTC day. The HTTP layer already caches for 60s, so calling
// DayMatches multiple times within the same scrape loop is cheap.
//
// Limitation: the upstream /data/games endpoint only returns the
// current UTC day. ScrapeNext7Days iterates the next seven days
// against this source, so six of the seven iterations would hit
// the network just to discover "no data for that date". We
// short-circuit on a non-today UTC date and return an empty slice
// immediately, so only the today-UTC call costs a request. The
// trade-off is documented and intentional: when 365scores starts
// publishing per-day feeds this guard can be removed.
func (s *Source) DayMatches(ctx context.Context, date time.Time) ([]scraper.Match, error) {
	today := time.Now().UTC().Format("2006-01-02")
	if date.UTC().Format("2006-01-02") != today {
		return []scraper.Match{}, nil
	}
	feed, err := s.client.FetchGames(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("scores365: DayMatches fetch %s: %w", date.Format("2006-01-02"), err)
	}
	out := make([]scraper.Match, 0, len(feed.Games))
	for _, raw := range feed.Games {
		match, ok := s.eventToMatch(raw, feed.Competitions, feed.Countries)
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
// are managed via the admin catalog, mirroring the sportsdb source.
func (s *Source) SearchLeagues(_ context.Context, _ string) ([]scraper.LeagueSearchResult, error) {
	return nil, errors.New("scores365: SearchLeagues is not supported; add leagues via the catalog admin form")
}

// eventToMatch converts a wire Game into the scraper.Match shape.
// Returns (zero, false) when the game lacks essential fields.
func (s *Source) eventToMatch(raw scores365.Game, competitions []scores365.Competition, countries []scores365.Country) (scraper.Match, bool) {
	if raw.ID == 0 || raw.Comp == 0 {
		return scraper.Match{}, false
	}
	if len(raw.Comps) < 2 {
		return scraper.Match{}, false
	}
	home := raw.Comps[0]
	away := raw.Comps[1]
	if home.ID == 0 || away.ID == 0 {
		return scraper.Match{}, false
	}

	startTS, ok := parseSTime(raw.STime)
	if !ok {
		s.logger.Warn("scores365: skip game unparseable STime",
			slog.Int("game_id", raw.ID),
			slog.String("stime", raw.STime))
		return scraper.Match{}, false
	}

	score := parseScrs(raw.Scrs)
	leagueName, leagueCountry := lookupLeague(raw.Comp, competitions, countries)
	return scraper.Match{
		Source:         s.Name(),
		SourceMatchId:  "scores365-" + strconv.Itoa(raw.ID),
		StartTimestamp: startTS,
		HomeScore:      score.home,
		AwayScore:      score.away,
		Status:         statusFlags(raw, startTS, time.Now()),
		HomeTeam:       s.teamFromEvent(home),
		AwayTeam:       s.teamFromEvent(away),
		League: scraper.LeagueRef{
			Source:         s.Name(),
			SourceLeagueId: strconv.FormatInt(LeagueIDPrefix+int64(raw.Comp), 10),
			Name:           leagueName,
			Sport:          sportSlugFromSID(raw.SID),
			Country:        leagueCountry,
		},
	}, true
}

// lookupLeague resolves the human-readable league name and country
// from the upstream Feed slices. Both lookups are best-effort: a
// missing Competition yields empty Name; a missing Country yields
// empty Country. The dispatch path does not fill these from the
// catalog, so they must be set here for the persisted tournament
// row to be useful.
func lookupLeague(compID int, competitions []scores365.Competition, countries []scores365.Country) (string, string) {
	var comp *scores365.Competition
	for i := range competitions {
		if competitions[i].ID == compID {
			comp = &competitions[i]
			break
		}
	}
	if comp == nil {
		return "", ""
	}
	for i := range countries {
		if countries[i].ID == comp.CID {
			return comp.Name, countries[i].Name
		}
	}
	return comp.Name, ""
}

// teamFromEvent builds a scraper.Team from the upstream event's
// name, ID, and colors. The ID is prefixed with TeamIDPrefix so
// the row lands in the shared `teams` table without colliding with
// any FotMob-sourced or sportsdb-sourced row.
func (s *Source) teamFromEvent(t scores365.Team) scraper.Team {
	return scraper.Team{
		SourceId:       TeamIDPrefix + int64(t.ID),
		Name:           t.Name,
		PrimaryColor:   t.Color,
		SecondaryColor: t.Color2,
	}
}

type scorePair struct{ home, away int }

// parseScrs interprets the Scrs float array. For most sports it's
// [home, away]. For baseball it's per-inning; we use the LAST two
// entries (latest cumulative score). If the array is short, both
// halves default to 0.
func parseScrs(scrs []float64) scorePair {
	if len(scrs) < 2 {
		return scorePair{}
	}
	home, away := scrs[len(scrs)-2], scrs[len(scrs)-1]
	return scorePair{home: int(home), away: int(away)}
}

// parseSTime converts "13-09-2026 23:20" (DD-MM-YYYY HH:MM) to a
// time.Time in UTC. The wire format is fixed and does not carry
// timezone info, so we assume UTC.
func parseSTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("02-01-2006 15:04", s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// statusFlags returns the MatchStatus for a wire Game.
//
// The events repository queries against (Started, Finished,
// Cancelled) bools, not against the source-specific label, so we
// translate the 365scores GT code + Completion/ETime/STime into
// that triple here. The Type and Description fields are kept as
// source-specific labels for downstream consumers that want a
// human-readable status.
func statusFlags(raw scores365.Game, startTS time.Time, now time.Time) scraper.MatchStatus {
	st := scraper.MatchStatus{
		Code:        raw.GT,
		Description: statusMap[raw.GT],
	}
	switch raw.GT {
	case -1, 1, 4:
		// notstarted / scheduled / postponed
		st.Type = "NS"
		return st
	case 12, 13, 22, 23, 24, 25, 26, 75, 76, 77, 78, 79, 80, 8:
		// live + interrupted treated as live
		st.Started = true
		if raw.GT == 8 {
			st.Type = "INT"
		} else {
			st.Type = "LIVE"
		}
		return st
	case 3, 32, 33, 88, 9:
		// finished + walkover treated as finished
		st.Started = true
		st.Finished = true
		if raw.GT == 9 {
			st.Type = "WO"
		} else {
			st.Type = "FT"
		}
		return st
	case 5:
		st.Cancelled = true
		st.Type = "CANC"
		return st
	}

	// Unmapped GT: fall back to a Completion/ETime/STime heuristic.
	if raw.Completion != nil && *raw.Completion >= 100 {
		if endTS, ok := parseSTime(raw.ETime); ok && endTS.Before(now) {
			st.Started = true
			st.Finished = true
			st.Type = "FT"
			st.Description = "finished"
			return st
		}
		st.Started = true
		st.Type = "LIVE"
		st.Description = "live"
		return st
	}
	if !startTS.IsZero() && startTS.After(now) {
		st.Type = "NS"
		st.Description = "notstarted"
		return st
	}
	st.Started = true
	st.Type = "LIVE"
	st.Description = "live"
	return st
}
