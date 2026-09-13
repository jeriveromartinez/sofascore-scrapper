package fotmob

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

type Source struct {
	client *Client
	logger *slog.Logger
}

func NewSource(client *Client, logger *slog.Logger) *Source {
	if logger == nil {
		logger = slog.Default()
	}
	return &Source{client: client, logger: logger}
}

func (s *Source) Name() string { return "fotmob" }

// DayMatches fetches the entire per-day payload from FotMob in a
// single HTTP round-trip and converts every league group into
// per-league Match slices. Each Match's League.SourceLeagueId is set
// to the upstream id so Service can dispatch by the configured
// league list.
//
// The scheduler prefers DayMatches over ScheduledEvents because the
// /api/data/matches endpoint has no per-league filter — calling
// ScheduledEvents per league would issue one HTTP request per
// (league, date) pair. DayMatches is the P2 #1 dedup fix for PR
// #124: 1 round-trip per date instead of N.
func (s *Source) DayMatches(ctx context.Context, date time.Time) ([]scraper.Match, error) {
	resp, err := s.client.dayMatches(ctx, date)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0)
	for _, lg := range resp.Leagues {
		leagueRef := scraper.LeagueRef{
			Source:         s.Name(),
			SourceLeagueId: strconv.FormatInt(lg.Id, 10),
			Name:           lg.Name,
			// Codex P1 on PR #128: the upstream payload only carries
			// the Ccode (e.g. "ENG", "INT"); the rest of the system
			// expects a non-empty Country on every Match. Copy the
			// upstream Ccode verbatim — it is already the short
			// code the scraper uses (e.g. "GB" for "England" from
			// the curated seed was a coincidence; Ccode is the
			// canonical FotMob short form).
			Country: lg.Ccode,
			Sport:   "football",
		}
		for _, m := range lg.Matches {
			out = append(out, s.toMatch(m, leagueRef))
		}
	}
	return out, nil
}

func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	matches, err := s.DayMatches(ctx, date)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0)
	for _, m := range matches {
		if m.League.SourceLeagueId == league.SourceLeagueId {
			out = append(out, m)
		}
	}
	return out, nil
}

// toMatch maps a single FotMob apiMatch to the source-agnostic
// scraper.Match. PR #124:
//
//   - The legacy /api/leagues fields (ImageUrl/PrimaryColor/
//     SecondaryColor/TextColor on teams, the embedded timeUtc on
//     a struct, the slug) are gone from the wire. We map whatever
//     FotMob still sends (id, name) and leave the dropped fields
//     at their zero value.
//   - HomeScore/AwayScore come from the per-team int fields.
//     Status.ScoreStr is preserved for backwards compatibility but
//     ToEvent reads the int halves directly.
//   - Status.Type is taken from status.reason.short when available
//     so the UI can label matches ("FT", "HT", "NS", …) without
//     needing its own FotMob-specific mapping.
//   - StartTimestamp is parsed from status.utcTime (RFC3339Nano in
//     the real payload).
func (s *Source) toMatch(m apiMatch, league scraper.LeagueRef) scraper.Match {
	var start time.Time
	if m.Status.UtcTime != "" {
		if t, err := time.Parse(time.RFC3339, m.Status.UtcTime); err == nil {
			start = t
		}
	}

	var statusType string
	if m.Status.Reason != nil {
		if v, ok := m.Status.Reason["short"].(string); ok {
			statusType = v
		}
	}

	return scraper.Match{
		Source:         s.Name(),
		SourceMatchId:  strconv.FormatInt(m.Id, 10),
		Slug:           m.Slug(),
		StartTimestamp: start,
		HomeScore:      m.Home.Score,
		AwayScore:      m.Away.Score,
		Status: scraper.MatchStatus{
			Code:      m.StatusId,
			Type:      statusType,
			Finished:  m.Status.Finished,
			Started:   m.Status.Started,
			Cancelled: m.Status.Cancelled,
			ScoreStr:  m.Status.ScoreStr,
		},
		HomeTeam: scraper.Team{
			SourceId: m.Home.Id,
			Name:     m.Home.Name,
		},
		AwayTeam: scraper.Team{
			SourceId: m.Away.Id,
			Name:     m.Away.Name,
		},
		League: league,
	}
}

func (s *Source) SearchLeagues(ctx context.Context, query string) ([]scraper.LeagueSearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	resp, err := s.client.Suggest(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("fotmob: suggest %q: %w", q, err)
	}
	out := make([]scraper.LeagueSearchResult, 0, len(resp.Suggestions))
	for _, e := range resp.Suggestions {
		if !strings.EqualFold(e.Type, "league") {
			continue
		}
		if e.Id == 0 {
			continue
		}
		out = append(out, scraper.LeagueSearchResult{
			Source:         s.Name(),
			SourceLeagueId: strconv.FormatInt(e.Id, 10),
			Name:           e.Name,
			Country:        strings.ToUpper(strings.TrimSpace(e.Country)),
			Sport:          e.Sport,
		})
	}
	return out, nil
}

// Slug is a small helper that builds a slug from the human label
// of the match. The /api/data/matches payload does not ship a
// `slug` field the way /api/leagues did, but the Event.Slug column
// is still useful for the admin UI ("team-a-vs-team-b" makes the
// URL readable). We synthesize one from the team long names when
// available, falling back to shortName.
func (m apiMatch) Slug() string {
	home := pickName(m.Home.LongName, m.Home.ShortName, m.Home.Name)
	away := pickName(m.Away.LongName, m.Away.ShortName, m.Away.Name)
	if home == "" || away == "" {
		return ""
	}
	return slugify(home) + "-vs-" + slugify(away)
}

func pickName(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func slugify(s string) string {
	const fallback = "-"
	var b []byte
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b = append(b, byte(r))
			prevDash = false
		case r >= 'A' && r <= 'Z':
			b = append(b, byte(r+('a'-'A')))
			prevDash = false
		case r >= '0' && r <= '9':
			b = append(b, byte(r))
			prevDash = false
		default:
			if !prevDash && len(b) > 0 {
				b = append(b, '-')
				prevDash = true
			}
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return fallback
	}
	return string(b)
}

var _ scraper.Source = (*Source)(nil)
