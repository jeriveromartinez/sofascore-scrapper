// internal/scraper/catalog/repository.go
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
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
		// MySQL reports 0 affected rows for an UPDATE that changes
		// nothing, so the row's existence has to be checked
		// separately before the update is called a miss.
		var count int64
		if err := r.db.WithContext(ctx).Model(&ScraperLeague{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
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
		q = q.Where("name LIKE ? ESCAPE '!'", "%"+escapeLike(f.Query)+"%")
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
// league goes to the source that published its id.
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

// ActiveLeaguesBySource returns the enabled leagues that belong to the
// requested source.
//
// The league id is only meaningful inside the source that published
// it, so the row's own source has to match: routing a league to
// another dispatcher would look its id up in a foreign id space and
// silently match a different competition. An override_source that
// names another source is therefore ignored here, exactly like the
// ActiveLeagues predicate does.
func (r *Repository) ActiveLeaguesBySource(ctx context.Context, source string) ([]scraper.LeagueRef, error) {
	var rows []ScraperLeague
	q := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Where("source = ? AND (override_source IS NULL OR override_source = ?)", source, source)
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

// EnsureLeague upserts a league row for the given
// (source, source_league_id) pair. Returns (true, nil) when a
// new row was inserted and (false, nil) when a row already
// existed (regardless of its enabled flag — the operator may
// have disabled it on a previous tick).
//
// The bool return exists so the dispatch loop can avoid
// re-upserting matches for leagues the operator has disabled:
// if the row was already there, we do not push fresh events
// into it. Without this guard a disabled scores365 league
// would silently start collecting matches again on every
// dispatch tick, defeating the operator's opt-out.
//
// Used by the scraper dispatch loop when a bulk-fetch source
// (currently scores365) returns matches for a league that is
// not yet in the catalog. Without this the bulk path would
// silently drop every new league the upstream picks up; with
// this the admin sees the new leagues on the next dashboard
// load and can disable unwanted ones.
func (r *Repository) EnsureLeague(ctx context.Context, sourceLeagueID string, league scraper.LeagueRef) (bool, error) {
	if sourceLeagueID == "" {
		return false, errors.New("catalog: EnsureLeague requires source_league_id")
	}
	source := league.Source
	if source == "" {
		return false, errors.New("catalog: EnsureLeague requires league.Source")
	}
	// Cheap pre-check so we don't fire a SELECT FOR UPDATE on
	// every dispatch tick.
	var existing ScraperLeague
	err := r.db.WithContext(ctx).
		Where("source = ? AND source_league_id = ?", source, sourceLeagueID).
		First(&existing).Error
	if err == nil {
		// Row exists, possibly disabled by the operator. We
		// report !created so the dispatch loop skips the
		// upsert step for this league in this branch.
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	row := &ScraperLeague{
		Source:         source,
		SourceLeagueId: sourceLeagueID,
		Name:           league.Name,
		Sport:          league.Sport,
		Country:        truncateCountry(league.Country),
		Enabled:        true,
	}
	// The unique index is the arbiter of the race. The insert cannot
	// be turned into a no-op clause instead: MariaDB answers a
	// conflicting INSERT ... ON DUPLICATE KEY UPDATE with the
	// existing row, so the affected-row count stays at one and the
	// caller could no longer tell a fresh insert from a race.
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		if isUniqueViolation(err) {
			// Raced with another worker; treat as success but
			// not as a fresh insert.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SearchLocalByName is a free-text match against the Name
// column. The match is case-insensitive and the term may appear
// anywhere in the name; only live (non-soft-deleted) rows are
// returned. Used by the admin catalog search box; the backend
// keeps its scope narrow (LIKE %q%) so it works without
// indexes and is safe to call on every keystroke.
func (r *Repository) SearchLocalByName(ctx context.Context, query string) ([]scraper.LeagueRef, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	var rows []ScraperLeague
	if err := r.db.WithContext(ctx).
		Where("LOWER(name) LIKE ? ESCAPE '!'", "%"+escapeLike(strings.ToLower(q))+"%").
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

// escapeLike escapes the characters special to SQL LIKE (% multi-char
// wildcard, _ single-char wildcard, ! escape char) by prefixing each with
// the escape character. Backslash is literal under ESCAPE '!', the clause
// every query here uses so MariaDB and SQLite agree. Without this, an admin
// entering Query=% widens the search to every row in the catalog table, and
// Query=_ matches every single-character name — both wildcard-injection
// primitives that the events package already protects against. Kept here
// (rather than promoted to a shared util) to avoid a cross-package import
// for a 12-line helper used in two places.
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '!' || r == '%' || r == '_' {
			b.WriteByte('!')
		}
		b.WriteRune(r)
	}
	return b.String()
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

// countryColumnSize mirrors ScraperLeague.Country's `size:8` column.
const countryColumnSize = 8

// truncateCountry bounds a country label to what the column can hold.
// Sources disagree on what they publish here: FotMob sends a short
// code ("ENG"), while the 365scores competitions API only exposes a
// full country name ("Argentina"). Writing the longer value straight
// through makes MariaDB reject the INSERT in strict mode, which would
// abort a whole discovery sweep over one row, so the value is clipped
// instead. The full, untruncated name still reaches tournaments.Region
// (a longtext column) via scraper.ToTournament.
func truncateCountry(country string) string {
	country = strings.TrimSpace(country)
	if len([]rune(country)) <= countryColumnSize {
		return country
	}
	return string([]rune(country)[:countryColumnSize])
}
