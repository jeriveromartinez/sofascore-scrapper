package catalog

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder keeps every executed statement so the tests can pin the
// LIKE escape character the repository sends to the database.
type sqlRecorder struct {
	mu    sync.Mutex
	stmts []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.stmts = append(r.stmts, sql)
	r.mu.Unlock()
}

func (r *sqlRecorder) containing(substr string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, s := range r.stmts {
		if strings.Contains(s, substr) {
			out = append(out, s)
		}
	}
	return out
}

func (r *sqlRecorder) reset() {
	r.mu.Lock()
	r.stmts = nil
	r.mu.Unlock()
}

func assertPortableLikeEscape(t *testing.T, rec *sqlRecorder) {
	t.Helper()
	stmts := rec.containing("LIKE")
	if len(stmts) == 0 {
		t.Fatal("no LIKE query captured")
	}
	for _, stmt := range stmts {
		if !strings.Contains(stmt, "ESCAPE '!'") {
			t.Fatalf("LIKE query not portable to MariaDB (want ESCAPE '!'): %s", stmt)
		}
		if strings.Contains(stmt, "ESCAPE '\\'") {
			t.Fatalf("LIKE query still uses the backslash escape character: %s", stmt)
		}
	}
}

func TestListUsesPortableLikeEscape(t *testing.T) {
	db := newTestDB(t)
	rec := &sqlRecorder{}
	tracked := db.Session(&gorm.Session{Logger: rec})
	repo := NewRepository(tracked)
	if err := repo.Create(context.Background(), &ScraperLeague{
		Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Enabled: true,
	}); err != nil {
		t.Fatalf("seed league: %v", err)
	}

	rec.reset()
	rows, total, err := repo.List(context.Background(), ListFilters{Query: "Premier"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List returned %d rows (total %d), want 1", len(rows), total)
	}
	assertPortableLikeEscape(t, rec)
}

func TestSearchLocalByNameUsesPortableLikeEscape(t *testing.T) {
	db := newTestDB(t)
	rec := &sqlRecorder{}
	tracked := db.Session(&gorm.Session{Logger: rec})
	repo := NewRepository(tracked)
	if err := repo.Create(context.Background(), &ScraperLeague{
		Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Enabled: true,
	}); err != nil {
		t.Fatalf("seed league: %v", err)
	}

	rec.reset()
	rows, err := repo.SearchLocalByName(context.Background(), "Premier")
	if err != nil {
		t.Fatalf("SearchLocalByName: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("SearchLocalByName returned %d rows, want 1", len(rows))
	}
	assertPortableLikeEscape(t, rec)
}

func TestEscapeLikeUsesBangAsEscapeChar(t *testing.T) {
	cases := map[string]string{
		"%":       "!%",
		"_":       "!_",
		"!":       "!!",
		`\`:       `\`,
		"a%b_c!d": "a!%b!_c!!d",
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
