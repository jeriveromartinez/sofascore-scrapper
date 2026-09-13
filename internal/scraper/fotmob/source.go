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

func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	apiMatches, err := s.client.ScheduledEvents(ctx, league.SourceLeagueId, date.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0, len(apiMatches))
	for _, m := range apiMatches {
		start, _ := time.Parse(time.RFC3339, m.Time.UtcTime)
		out = append(out, scraper.Match{
			Source:         s.Name(),
			SourceMatchId:  m.Id,
			Slug:           m.Slug,
			StartTimestamp: start,
			Status: scraper.MatchStatus{
				Code:      m.Status.Code,
				Type:      m.Status.Type,
				Finished:  m.Status.Finished,
				Started:   m.Status.Started,
				Cancelled: m.Status.Cancelled,
				ScoreStr:  m.Status.ScoreStr,
			},
			HomeTeam: scraper.Team{
				SourceId:       m.Home.Id,
				Name:           m.Home.Name,
				LogoURL:        m.Home.ImageUrl,
				PrimaryColor:   m.Home.PrimaryColor,
				SecondaryColor: m.Home.SecondaryColor,
				TextColor:      m.Home.TextColor,
			},
			AwayTeam: scraper.Team{
				SourceId:       m.Away.Id,
				Name:           m.Away.Name,
				LogoURL:        m.Away.ImageUrl,
				PrimaryColor:   m.Away.PrimaryColor,
				SecondaryColor: m.Away.SecondaryColor,
				TextColor:      m.Away.TextColor,
			},
			League: league,
		})
	}
	return out, nil
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

var _ scraper.Source = (*Source)(nil)
