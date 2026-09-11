// internal/scraper/catalog/service.go
package catalog

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

type SourceSearcher interface {
	SearchLeagues(ctx context.Context, query string) ([]scraper.LeagueSearchResult, error)
}

type Service struct {
	repo   *Repository
	search SourceSearcher
	logger *slog.Logger
}

func NewService(repo *Repository, search SourceSearcher) *Service {
	return &Service{repo: repo, search: search, logger: slog.Default()}
}

func (s *Service) Create(ctx context.Context, sl *ScraperLeague) error {
	sl.Source = strings.TrimSpace(sl.Source)
	if sl.Source == "" {
		sl.Source = "fotmob"
	}
	sl.SourceLeagueId = strings.TrimSpace(sl.SourceLeagueId)
	sl.Name = strings.TrimSpace(sl.Name)
	sl.Country = strings.ToUpper(strings.TrimSpace(sl.Country))
	if sl.Sport == "" {
		sl.Sport = "football"
	}
	if err := validate(sl); err != nil {
		return err
	}
	return s.repo.Create(ctx, sl)
}

func (s *Service) Update(ctx context.Context, id uint, fields map[string]any) error {
	return s.repo.Update(ctx, id, fields)
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	return s.repo.SoftDelete(ctx, id)
}

func (s *Service) Get(ctx context.Context, id uint) (*ScraperLeague, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f ListFilters) ([]ScraperLeague, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) ActiveLeagues(ctx context.Context) ([]scraper.LeagueRef, error) {
	return s.repo.ActiveLeagues(ctx)
}

func (s *Service) SearchLeagues(ctx context.Context, query string) ([]scraper.LeagueSearchResult, error) {
	if s.search == nil {
		return nil, nil
	}
	return s.search.SearchLeagues(ctx, query)
}

func validate(sl *ScraperLeague) error {
	if sl.Source != "fotmob" {
		return errors.New("catalog: source must be 'fotmob' in v1")
	}
	if sl.SourceLeagueId == "" {
		return errors.New("catalog: source_league_id required")
	}
	if sl.Name == "" {
		return errors.New("catalog: name required")
	}
	return nil
}
