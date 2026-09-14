// internal/scraper/catalog/repository.go
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("catalog: not found")
var ErrDuplicate = errors.New("catalog: duplicate (source, source_league_id)")

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, sl *ScraperLeague) error {
	if err := r.db.WithContext(ctx).Create(sl).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s/%s", ErrDuplicate, sl.Source, sl.SourceLeagueId)
		}
		return err
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, id uint, fields map[string]any) error {
	res := r.db.WithContext(ctx).Model(&ScraperLeague{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uint) (*ScraperLeague, error) {
	var sl ScraperLeague
	if err := r.db.WithContext(ctx).First(&sl, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sl, nil
}

type ListFilters struct {
	Enabled *bool
	Source  string
	Country string
	Query   string
	Page    int
	Limit   int
}

func (r *Repository) List(ctx context.Context, f ListFilters) ([]ScraperLeague, int64, error) {
	q := r.db.WithContext(ctx).Model(&ScraperLeague{})
	if f.Enabled != nil {
		q = q.Where("enabled = ?", *f.Enabled)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.Country != "" {
		q = q.Where("country = ?", f.Country)
	}
	if f.Query != "" {
		q = q.Where("name LIKE ?", "%"+f.Query+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page < 1 {
		f.Page = 1
	}
	var out []ScraperLeague
	if err := q.Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Order("name ASC").Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) SoftDelete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&ScraperLeague{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ActiveLeagues is the multi-source dispatcher default: every enabled
// league goes to its natural source unless override_source is set.
func (r *Repository) ActiveLeagues(ctx context.Context) ([]scraper.LeagueRef, error) {
	var rows []ScraperLeague
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("override_source IS NULL OR override_source = source").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]scraper.LeagueRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, scraper.LeagueRef{
			Source:         r.Source,
			SourceLeagueId: r.SourceLeagueId,
			Name:           r.Name,
			Country:        r.Country,
			Sport:          r.Sport,
		})
	}
	return out, nil
}

// ActiveLeaguesBySource returns enabled leagues whose effective
// source matches the requested name. The effective source is
// COALESCE(override_source, source): a row with
// source=scores365, override_source=fotmob belongs to the fotmob
// dispatcher, NOT the scores365 one. Used by the dispatcher to
// route per-league.
func (r *Repository) ActiveLeaguesBySource(ctx context.Context, source string) ([]scraper.LeagueRef, error) {
	var rows []ScraperLeague
	q := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("(source = ? AND override_source IS NULL) OR override_source = ?", source, source)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]scraper.LeagueRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, scraper.LeagueRef{
			Source:         r.Source,
			SourceLeagueId: r.SourceLeagueId,
			Name:           r.Name,
			Sport:          r.Sport,
			Country:        r.Country,
		})
	}
	return out, nil
}

// SearchLocalByName looks up entries already in scraper_leagues
// whose name matches the search term. It returns only live (non-soft-
// deleted) rows. Used by Service.SearchLeagues to prefer the
// verified-on-this-deployment IDs over the upstream FotMob search
// results, which can be stale. The match is case-insensitive and
// requires the term to appear anywhere in the name.
// EnsureLeague upserts a league row for the given
// (source, source_league_id) pair. Idempotent: if the row
// already exists it is left untouched, so operator-set
// enabled flags and curated name overrides are preserved.
//
// Used by the scraper dispatch loop when a bulk-fetch source
// (TheSportsDB) returns matches for a league that is not yet
// in the catalog. Without this the bulk path would silently
// drop every new league TheSportsDB picks up; with this the
// admin sees the new leagues on the next dashboard load and
// can disable unwanted ones.
func (r *Repository) EnsureLeague(ctx context.Context, sourceLeagueID string, league scraper.LeagueRef) error {
	if sourceLeagueID == "" {
		return errors.New("catalog: EnsureLeague requires source_league_id")
	}
	source := league.Source
	if source == "" {
		return errors.New("catalog: EnsureLeague requires league.Source")
	}
	// Cheap pre-check so we don't fire a SELECT FOR UPDATE on
	// every dispatch tick.
	var existing ScraperLeague
	err := r.db.WithContext(ctx).
		Where("source = ? AND source_league_id = ?", source, sourceLeagueID).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	row := &ScraperLeague{
		Source:         source,
		SourceLeagueId: sourceLeagueID,
		Name:           league.Name,
		Sport:          league.Sport,
		Country:        league.Country,
		Enabled:        true,
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		if isUniqueViolation(err) {
			return nil // raced with another worker; treat as success
		}
		return err
	}
	return nil
}

// ExistsBySourceLeagueID returns 1 if a row with the given (source,
// source_league_id) exists in scraper_leagues (excluding soft-deleted),
// else 0. Errors propagate.
func (r *Repository) ExistsBySourceLeagueID(ctx context.Context, source, sourceLeagueID string) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&ScraperLeague{}).
		Where("source = ? AND source_league_id = ?", source, sourceLeagueID).
		Where("deleted_at IS NULL").
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// SearchLocalByName is a free-text match against the Name
// column. Used by the admin catalog search box; the backend
// keeps its scope narrow (LIKE %q%) so it works without
// indexes and is safe to call on every keystroke.
func (r *Repository) SearchLocalByName(ctx context.Context, query string) ([]scraper.LeagueRef, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	var rows []ScraperLeague
	if err := r.db.WithContext(ctx).
		Where("LOWER(name) LIKE ?", "%"+strings.ToLower(q)+"%").
		Order("name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]scraper.LeagueRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, scraper.LeagueRef{
			Source:         r.Source,
			SourceLeagueId: r.SourceLeagueId,
			Name:           r.Name,
			Country:        r.Country,
			Sport:          r.Sport,
		})
	}
	return out, nil
}

func isUniqueViolation(err error) bool {
	// MariaDB: error 1062
	return err != nil && (containsAll(err.Error(), "Error 1062") || containsAll(err.Error(), "UNIQUE constraint failed"))
}

func containsAll(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
