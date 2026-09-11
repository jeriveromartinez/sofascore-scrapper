package scraper

import "context"

// CatalogSource provides the active leagues to scrape. The DB-backed
// implementation lives in internal/scraper/catalog (PR 3). This stub
// is used by PR 2 so the Service can be wired before the DB CRUD lands.
type CatalogSource interface {
	ActiveLeagues(ctx context.Context) ([]LeagueRef, error)
}

type memoryCatalog struct {
	leagues []LeagueRef
}

func NewMemoryCatalog(leagues []LeagueRef) CatalogSource {
	return &memoryCatalog{leagues: leagues}
}

func (m *memoryCatalog) ActiveLeagues(_ context.Context) ([]LeagueRef, error) {
	out := make([]LeagueRef, len(m.leagues))
	copy(out, m.leagues)
	return out, nil
}
