package fotmob

import (
	"context"
	"log/slog"
	"strconv"
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

func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	apiMatches, err := s.client.ScheduledEvents(ctx, league.SourceLeagueId, date)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0, len(apiMatches))
	for _, m := range apiMatches {
		out = append(out, s.toMatch(m, league))
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
	// Implementación contra /api/searchapi/suggest?term=...
	// Se agrega en PR 3 (cuando el admin UI lo necesite).
	return nil, nil
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
