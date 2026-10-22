// internal/scraper/catalog/service.go
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
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
	sl.Country = truncateCountry(strings.ToUpper(strings.TrimSpace(sl.Country)))
	if sl.Sport == "" {
		sl.Sport = "football"
	}
	override, err := normalizeOverride(sl.OverrideSource)
	if err != nil {
		return err
	}
	sl.OverrideSource = override
	if err := validate(sl); err != nil {
		return err
	}
	return s.repo.Create(ctx, sl)
}

func (s *Service) Update(ctx context.Context, id uint, fields map[string]any) error {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrNotFound
	}
	merged, updates, err := mergeUpdate(current, fields)
	if err != nil {
		return err
	}
	if err := validate(merged); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, updates)
}

// mergeUpdate applies the patch to the stored row. It returns the
// merged row (which validate checks) and the values to persist.
//
// The persisted values are the normalised ones, not the raw wire
// values: an override_source of "" means "no override" and has to be
// written as NULL. Storing the empty string instead would leave the
// row outside the ActiveLeagues predicate, which only accepts NULL
// or the row's own source, and the league would silently stop being
// scraped.
func mergeUpdate(current *ScraperLeague, fields map[string]any) (*ScraperLeague, map[string]any, error) {
	merged := *current
	updates := map[string]any{}

	if raw, ok := fields["name"]; ok {
		name, ok := raw.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: name must be a string", ErrValidation)
		}
		merged.Name = strings.TrimSpace(name)
		updates["name"] = merged.Name
	}
	if raw, ok := fields["country"]; ok {
		country, ok := raw.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: country must be a string", ErrValidation)
		}
		merged.Country = truncateCountry(strings.ToUpper(strings.TrimSpace(country)))
		updates["country"] = merged.Country
	}
	if raw, ok := fields["sport"]; ok {
		sport, ok := raw.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: sport must be a string", ErrValidation)
		}
		merged.Sport = strings.TrimSpace(sport)
		if merged.Sport == "" {
			merged.Sport = "football"
		}
		updates["sport"] = merged.Sport
	}
	if raw, ok := fields["enabled"]; ok {
		enabled, ok := raw.(bool)
		if !ok {
			return nil, nil, fmt.Errorf("%w: enabled must be a boolean", ErrValidation)
		}
		merged.Enabled = enabled
		updates["enabled"] = enabled
	}
	if raw, ok := fields["override_source"]; ok {
		override, err := normalizeOverride(raw)
		if err != nil {
			return nil, nil, err
		}
		merged.OverrideSource = override
		if override == nil {
			updates["override_source"] = nil
		} else {
			updates["override_source"] = *override
		}
	}
	return &merged, updates, nil
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
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	// Prefer the verified-on-this-deployment catalog: any
	// scraper_leagues row whose name matches the term has a
	// source_league_id that the operator already knows scrapes
	// (or knows doesn't, and can disable). Returning the upstream
	// FotMob suggest list first would surface stale IDs (e.g. 48
	// for EFL Championship instead of the verified 938218) and the
	// admin who posted that ID would get nothing forever.
	if local, err := s.repo.SearchLocalByName(ctx, q); err != nil {
		return nil, err
	} else if len(local) > 0 {
		out := make([]scraper.LeagueSearchResult, 0, len(local))
		for _, lg := range local {
			out = append(out, scraper.LeagueSearchResult(lg))
		}
		return out, nil
	}
	if s.search == nil {
		return nil, nil
	}
	return s.search.SearchLeagues(ctx, q)
}

// knownSources are the scraper sources a catalog row may name.
// FotMob owns football; 365scores owns every other sport.
var knownSources = map[string]bool{
	"fotmob":    true,
	"scores365": true,
}

// ErrValidation marks the errors that come from the row itself
// rather than from the database, so the API can answer with a 400.
var ErrValidation = errors.New("catalog: validation failed")

func validate(sl *ScraperLeague) error {
	if !knownSources[sl.Source] {
		return fmt.Errorf("%w: source must be one of 'fotmob', 'scores365'", ErrValidation)
	}
	if sl.SourceLeagueId == "" {
		return fmt.Errorf("%w: source_league_id required", ErrValidation)
	}
	if sl.Name == "" {
		return fmt.Errorf("%w: name required", ErrValidation)
	}
	if sl.OverrideSource != nil && *sl.OverrideSource != sl.Source {
		return fmt.Errorf("%w: override_source must match source; a league id only means something inside its own source", ErrValidation)
	}
	return nil
}

// normalizeOverride turns the wire value of override_source into the
// stored form: nil for "no override", the trimmed source name
// otherwise.
func normalizeOverride(v any) (*string, error) {
	switch value := v.(type) {
	case nil:
		return nil, nil
	case *string:
		if value == nil {
			return nil, nil
		}
		return normalizeOverride(*value)
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, nil
		}
		return &trimmed, nil
	default:
		return nil, fmt.Errorf("%w: override_source must be a string", ErrValidation)
	}
}
