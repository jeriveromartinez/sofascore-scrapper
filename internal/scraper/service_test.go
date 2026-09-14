//go:build integration

package scraper

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

func setupScraperTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	// The full AutoMigrateAll lives in internal/platform/database but
	// importing it here creates an import cycle (catalog -> scraper).
	// Migrate the subset that UpsertScrapeBatch actually touches.
	if err := db.AutoMigrate(
		&events.Event{},
		&events.Team{},
		&tournaments.Tournament{},
		&tournaments.DeviceTournament{},
		&tournaments.GlobalTournamentConfig{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// Legacy tests preserved as documentation for what Task 7 will delete.
// They reference the removed SofaScore types (APIEvent, TeamApi) and the
// old ToScrapeBatch signature; the bodies are kept here as block comments
// so future readers can see the original test intent.

func TestToScrapeBatch_Deduplication(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		home := TeamApi{ID: 1, Name: "Home"}
		away := TeamApi{ID: 2, Name: "Away"}

		apiEvents := []*APIEvent{
			{
				ID:             100,
				Slug:           "match-1",
				StartTimestamp: 1710000000,
				HomeTeam:       home,
				AwayTeam:       away,
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
			{
				ID:             101,
				Slug:           "match-2",
				StartTimestamp: 1710000000,
				HomeTeam:       home,
				AwayTeam:       away,
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
		}
		apiEvents[0].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[0].Tournament.UniqueTournament.ID = 1
		apiEvents[0].Tournament.UniqueTournament.Name = "Test"
		apiEvents[0].Tournament.UniqueTournament.Slug = "test"
		apiEvents[0].Tournament.UniqueTournament.Category.Name = "Category"
		apiEvents[0].Tournament.UniqueTournament.Category.Slug = "category"

		apiEvents[1].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[1].Tournament.UniqueTournament.ID = 1
		apiEvents[1].Tournament.UniqueTournament.Name = "Test"
		apiEvents[1].Tournament.UniqueTournament.Slug = "test"
		apiEvents[1].Tournament.UniqueTournament.Category.Name = "Category"
		apiEvents[1].Tournament.UniqueTournament.Category.Slug = "category"

		batch := ToScrapeBatch(apiEvents, "football")

		if len(batch.Teams) != 2 {
			t.Errorf("should deduplicate 4 team references to 2, got %d", len(batch.Teams))
		}
		if len(batch.Tournaments) != 1 {
			t.Errorf("should deduplicate 2 tournament references to 1, got %d", len(batch.Tournaments))
		}
		if len(batch.Events) != 2 {
			t.Errorf("should have 2 events, got %d", len(batch.Events))
		}

		for i := range batch.Events {
			if batch.Events[i].HomeTeamModel != nil {
				t.Error("event should not have HomeTeamModel populated")
			}
			if batch.Events[i].AwayTeamModel != nil {
				t.Error("event should not have AwayTeamModel populated")
			}
			if batch.Events[i].League != nil {
				t.Error("event should not have League populated")
			}
		}
	*/
}

func TestToScrapeBatch_SortedOutput(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		apiEvents := []*APIEvent{
			{
				ID:             300,
				Slug:           "z-match",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 30, Name: "Z-Home"},
				AwayTeam:       TeamApi{ID: 10, Name: "A-Away"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "notstarted"},
			},
			{
				ID:             100,
				Slug:           "a-match",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 20, Name: "B-Home"},
				AwayTeam:       TeamApi{ID: 10, Name: "A-Away"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "notstarted"},
			},
		}
		for i := range apiEvents {
			apiEvents[i].Time.CurrentPeriodStartTimestamp = 1710000000
			apiEvents[i].Tournament.UniqueTournament.ID = 2
			apiEvents[i].Tournament.UniqueTournament.Name = "T"
			apiEvents[i].Tournament.UniqueTournament.Slug = "t"
			apiEvents[i].Tournament.UniqueTournament.Category.Name = "c"
			apiEvents[i].Tournament.UniqueTournament.Category.Slug = "c"
		}

		batch := ToScrapeBatch(apiEvents, "football")

		for i := 1; i < len(batch.Teams); i++ {
			if batch.Teams[i].TeamId < batch.Teams[i-1].TeamId {
				t.Errorf("teams not sorted by TeamId")
			}
		}
		for i := 1; i < len(batch.Events); i++ {
			if batch.Events[i].SofaScoreEventId < batch.Events[i-1].SofaScoreEventId {
				t.Errorf("events not sorted by SofaScoreEventId")
			}
		}
	*/
}

func TestToScrapeBatch_StructureRoundTrip(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		db := setupScraperTestDB(t)
		repo := events.NewRepository(db)

		apiEvents := []*APIEvent{
			{
				ID:             1,
				Slug:           "round-trip",
				StartTimestamp: 1710000000,
				HomeTeam:       TeamApi{ID: 1, Name: "H"},
				AwayTeam:       TeamApi{ID: 2, Name: "A"},
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: "inprogress"},
			},
		}
		apiEvents[0].Time.CurrentPeriodStartTimestamp = 1710000000
		apiEvents[0].Tournament.UniqueTournament.ID = 1
		apiEvents[0].Tournament.UniqueTournament.Name = "League"
		apiEvents[0].Tournament.UniqueTournament.Slug = "league"
		apiEvents[0].Tournament.UniqueTournament.Category.Name = "cat"
		apiEvents[0].Tournament.UniqueTournament.Category.Slug = "cat"

		batch := ToScrapeBatch(apiEvents, "football")

		if err := repo.UpsertScrapeBatch(nil, batch, 500); err != nil {
			t.Fatalf("UpsertScrapeBatch: %v", err)
		}

		var teamCount int64
		db.Model(&events.Team{}).Count(&teamCount)
		if teamCount != 2 {
			t.Errorf("expected 2 teams, got %d", teamCount)
		}

		var eventCount int64
		db.Model(&events.Event{}).Count(&eventCount)
		if eventCount != 1 {
			t.Errorf("expected 1 event, got %d", eventCount)
		}

		var tourCount int64
		db.Model(&tournaments.Tournament{}).Count(&tourCount)
		if tourCount != 1 {
			t.Errorf("expected 1 tournament, got %d", tourCount)
		}
	*/
}

func TestToScrapeBatch_EmptyInput(t *testing.T) {
	t.Skip("legacy; removed by Task 7")
	/*
		batch := ToScrapeBatch(nil, "football")

		if len(batch.Teams) != 0 {
			t.Errorf("expected 0 teams, got %d", len(batch.Teams))
		}
		if len(batch.Tournaments) != 0 {
			t.Errorf("expected 0 tournaments, got %d", len(batch.Tournaments))
		}
		if len(batch.Events) != 0 {
			t.Errorf("expected 0 events, got %d", len(batch.Events))
		}
	*/
}

type serviceTestFakeSource struct {
	matches []Match
	err     error
}

func (f *serviceTestFakeSource) Name() string { return "fake" }
func (f *serviceTestFakeSource) ScheduledEvents(_ context.Context, _ LeagueRef, _ time.Time) ([]Match, error) {
	return f.matches, f.err
}
func (f *serviceTestFakeSource) SearchLeagues(_ context.Context, _ string) ([]LeagueSearchResult, error) {
	return nil, nil
}

// dayMatchingFakeSource extends serviceTestFakeSource with the
// optional DayMatcher interface so the scheduler uses the day-dispatch
// path instead of per-league fetches. Used to verify the dedup
// behavior in TestService_ScrapeToday_DayMatcherDeduplicates and the
// auto-create path in TestService_ScrapeToday_AutoCreatesUnknownLeagues
// (which requires Name() == "scores365" to trigger the EnsureLeague
// guard inside dispatchDayMatch).
type dayMatchingFakeSource struct {
	name    string
	matches []Match
	err     error

	dayMatchesCalls atomic.Int64
	lastDate        time.Time
}

func (f *dayMatchingFakeSource) Name() string { return f.name }
func (f *dayMatchingFakeSource) DayMatches(_ context.Context, date time.Time) ([]Match, error) {
	f.dayMatchesCalls.Add(1)
	f.lastDate = date
	return f.matches, f.err
}
func (f *dayMatchingFakeSource) ScheduledEvents(_ context.Context, _ LeagueRef, _ time.Time) ([]Match, error) {
	return f.matches, f.err
}
func (f *dayMatchingFakeSource) SearchLeagues(_ context.Context, _ string) ([]LeagueSearchResult, error) {
	return nil, nil
}

// serviceTestFakeCatalog is a CatalogSource stub used by the integration
// test now that the memory-catalog package has been removed. It avoids
// an import cycle with internal/scraper/catalog (which imports this
// package for LeagueRef).
type serviceTestFakeCatalog struct {
	leagues []LeagueRef
}

func (c *serviceTestFakeCatalog) ActiveLeagues(_ context.Context) ([]LeagueRef, error) {
	out := make([]LeagueRef, len(c.leagues))
	copy(out, c.leagues)
	return out, nil
}

// ActiveLeaguesBySource returns the subset of leagues whose Source
// matches. The fixture only stores the natural Source — override_source
// routing is exercised by routingFakeCatalog in
// TestService_ScrapeToday_RoutesByOverrideSource below.
func (c *serviceTestFakeCatalog) ActiveLeaguesBySource(_ context.Context, source string) ([]LeagueRef, error) {
	out := make([]LeagueRef, 0, len(c.leagues))
	for _, l := range c.leagues {
		if l.Source == source {
			out = append(out, l)
		}
	}
	return out, nil
}

func TestService_ScrapeToday_UpsertsFromSource(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &serviceTestFakeSource{
		matches: []Match{
			{
				Source:         "fake",
				SourceMatchId:  "1",
				Slug:           "x",
				StartTimestamp: time.Now(),
				Status:         MatchStatus{Type: "scheduled"},
				HomeTeam:       Team{SourceId: 1, Name: "H"},
				AwayTeam:       Team{SourceId: 2, Name: "A"},
				League:         LeagueRef{Source: "fake", SourceLeagueId: "47", Name: "L"},
			},
		},
	}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{{Source: "fake", SourceLeagueId: "47", Name: "L"}}}
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 4, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	svc.ScrapeToday(context.Background(), time.Now())

	var count int64
	if err := db.Model(&events.Event{}).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event, got %d", count)
	}
}

// TestService_ScrapeToday_DayMatcherDeduplicates is the regression
// test for the P2 #1 Codex finding on PR #124: with N active
// leagues, ScrapeToday must issue exactly 1 HTTP round-trip (via
// DayMatches) instead of N. Without the dedup, a 41-league catalog
// fires 41 fetches per cron tick = ~59k fetches/day.
//
// The test uses a source that implements DayMatcher; the fake
// counts DayMatches invocations. The catalog has 3 active leagues;
// the assertion is that the counter is exactly 1 after ScrapeToday.
func TestService_ScrapeToday_DayMatcherDeduplicates(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &dayMatchingFakeSource{
		name: "scores365",
		matches: []Match{
			mkMatch("1", "47"),
			mkMatch("2", "87"),
			mkMatch("3", "54"),
		},
	}
	cat := &serviceTestFakeCatalog{leagues: []LeagueRef{
		{Source: "scores365", SourceLeagueId: "47", Name: "PL"},
		{Source: "scores365", SourceLeagueId: "87", Name: "LL"},
		{Source: "scores365", SourceLeagueId: "54", Name: "BL"},
	}}
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	if got := src.dayMatchesCalls.Load(); got != 1 {
		t.Errorf("DayMatches calls = %d, want 1 (3 active leagues should share 1 HTTP fetch)", got)
	}
	if !src.lastDate.Equal(fixedNow) {
		t.Errorf("DayMatches called with %v, want %v", src.lastDate, fixedNow)
	}

	// All three matches from the upstream payload land in the DB.
	var count int64
	if err := db.Model(&events.Event{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func mkMatch(id, leagueID string) Match {
	return Match{
		Source:         "scores365",
		SourceMatchId:  id,
		Slug:           "x-" + id,
		StartTimestamp: time.Now(),
		Status:         MatchStatus{Type: "scheduled"},
		HomeTeam:       Team{SourceId: 1, Name: "H"},
		AwayTeam:       Team{SourceId: 2, Name: "A"},
		League:         LeagueRef{Source: "scores365", SourceLeagueId: leagueID, Name: "L"},
	}
}

// ensureFakeCatalog is a CatalogSource that also implements
// EnsureLeague. It records every EnsureLeague call so the test
// can assert which league IDs the dispatcher picked up.
type ensureFakeCatalog struct {
	leagues []LeagueRef
	ensure  []string
}

func (c *ensureFakeCatalog) ActiveLeagues(_ context.Context) ([]LeagueRef, error) {
	out := make([]LeagueRef, len(c.leagues))
	copy(out, c.leagues)
	return out, nil
}

// ActiveLeaguesBySource mirrors serviceTestFakeCatalog: filter by
// the natural Source. The EnsureLeague auto-create path is
// exercised separately (TestService_ScrapeToday_AutoCreatesUnknownLeagues).
func (c *ensureFakeCatalog) ActiveLeaguesBySource(_ context.Context, source string) ([]LeagueRef, error) {
	out := make([]LeagueRef, 0, len(c.leagues))
	for _, l := range c.leagues {
		if l.Source == source {
			out = append(out, l)
		}
	}
	return out, nil
}

func (c *ensureFakeCatalog) EnsureLeague(_ context.Context, sourceLeagueID string, league LeagueRef) (bool, error) {
	for _, l := range c.ensure {
		if l == sourceLeagueID {
			return false, nil
		}
	}
	c.ensure = append(c.ensure, sourceLeagueID)
	return true, nil
}

// TestService_ScrapeToday_AutoCreatesUnknownLeagues covers the
// contract the discovery job relies on: the bulk-fetch
// dispatcher auto-creates any league ID it sees in the upstream
// payload that isn't in the catalog, so events stop being
// dropped once the upstream starts publishing a new league.
//
// Without EnsureLeague the dispatch loop would silently skip
// every match in leagues 87 and 54 (they're not in the active
// catalog), and the catalog table would never grow beyond the
// four seeded sportsdb leagues.
func TestService_ScrapeToday_AutoCreatesUnknownLeagues(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	src := &dayMatchingFakeSource{
		name: "scores365",
		matches: []Match{
			mkMatch("1", "47"), // known (active)
			mkMatch("2", "87"), // unknown
			mkMatch("3", "54"), // unknown
		},
	}
	cat := &ensureFakeCatalog{
		leagues: []LeagueRef{
			{Source: "scores365", SourceLeagueId: "47", Name: "PL"},
		},
	}
	svc, err := NewService(repo, NewSourceDispatcher(src), cat, 100, 1, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	// EnsureLeague must have been called for both unknown IDs
	// exactly once.
	want := map[string]bool{"87": false, "54": false}
	for _, id := range cat.ensure {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("EnsureLeague not called for %s", id)
		}
	}

	// All three matches persisted (known + auto-created buckets).
	var count int64
	if err := db.Model(&events.Event{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

// routingFakeRow stores both the natural Source and an optional
// override. The effective source for routing is
// OverrideSource if set, else Source.
type routingFakeRow struct {
	LeagueRef
	OverrideSource string
}

type routingFakeCatalog struct {
	rows []routingFakeRow
}

func (c *routingFakeCatalog) ActiveLeagues(_ context.Context) ([]LeagueRef, error) {
	out := make([]LeagueRef, 0, len(c.rows))
	for _, r := range c.rows {
		if r.OverrideSource != "" {
			// Pinned rows do not appear under their natural
			// source in the legacy bulk view, mirroring the
			// ActiveLeagues query in the DB repo.
			continue
		}
		out = append(out, r.LeagueRef)
	}
	return out, nil
}

func (c *routingFakeCatalog) ActiveLeaguesBySource(_ context.Context, source string) ([]LeagueRef, error) {
	out := make([]LeagueRef, 0, len(c.rows))
	for _, r := range c.rows {
		effective := r.Source
		if r.OverrideSource != "" {
			effective = r.OverrideSource
		}
		if effective == source {
			out = append(out, r.LeagueRef)
		}
	}
	return out, nil
}

// routingFakeSource records every league it is asked to scrape.
// It does NOT implement DayMatcher so the dispatcher uses the
// per-league path; this matches how the production scores365 and
// fotmob sources are wired (one HTTP call per league under the
// hood) and exercises the g.Go fan-out that the new
// per-source routing takes.
type routingFakeSource struct {
	name     string
	mu       sync.Mutex
	received []LeagueRef
}

func (f *routingFakeSource) Name() string { return f.name }
func (f *routingFakeSource) ScheduledEvents(_ context.Context, l LeagueRef, _ time.Time) ([]Match, error) {
	f.mu.Lock()
	f.received = append(f.received, l)
	f.mu.Unlock()
	return nil, nil
}
func (f *routingFakeSource) SearchLeagues(_ context.Context, _ string) ([]LeagueSearchResult, error) {
	return nil, nil
}

func (f *routingFakeSource) receivedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.received))
	for _, l := range f.received {
		out = append(out, l.SourceLeagueId)
	}
	return out
}

// TestService_ScrapeToday_RoutesByOverrideSource is the regression
// test for the P0 routing fix on PR #135: ScrapeToday must call
// ActiveLeaguesBySource per registered source so an operator-set
// override_source (which moves a league to a different dispatcher)
// is honoured instead of being mis-bucketed under its natural
// Source post-hoc.
//
// The catalog has three rows:
//   - {source: fotmob,    override: nil}            -> fotmob bucket
//   - {source: scores365, override: "fotmob"}       -> fotmob bucket (pinned AWAY)
//   - {source: fotmob,    override: "scores365"}    -> scores365 bucket (pinned TO)
//
// Before the fix, the third row went to the fotmob bucket (because
// groupBySource keyed by Source), so the scores365 bucket saw only
// its natural row and the fotmob bucket saw two of three rows.
// After the fix, the override-aware routing puts the pinned-TO
// row into the scores365 bucket.
func TestService_ScrapeToday_RoutesByOverrideSource(t *testing.T) {
	db := setupScraperTestDB(t)
	repo := events.NewRepository(db)

	fm := &routingFakeSource{name: "fotmob"}
	s365 := &routingFakeSource{name: "scores365"}
	dispatcher := NewSourceDispatcher(fm, s365)

	cat := &routingFakeCatalog{rows: []routingFakeRow{
		{LeagueRef: LeagueRef{Source: "fotmob", SourceLeagueId: "natural", Name: "NaturalFM"}},
		{LeagueRef: LeagueRef{Source: "scores365", SourceLeagueId: "pinned-away", Name: "PinnedAway"}, OverrideSource: "fotmob"},
		{LeagueRef: LeagueRef{Source: "fotmob", SourceLeagueId: "pinned-to", Name: "PinnedIn"}, OverrideSource: "scores365"},
	}}

	svc, err := NewService(repo, dispatcher, cat, 100, 1, slog.Default())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	fixedNow := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.ScrapeToday(context.Background(), fixedNow)

	fmIDs := fm.receivedIDs()
	s365IDs := s365.receivedIDs()

	// fotmob bucket: natural + pinned-away (2 total).
	if len(fmIDs) != 2 {
		t.Fatalf("fotmob bucket: want 2 leagues, got %d (%v)", len(fmIDs), fmIDs)
	}
	gotFM := map[string]bool{}
	for _, id := range fmIDs {
		gotFM[id] = true
	}
	if !gotFM["natural"] || !gotFM["pinned-away"] {
		t.Errorf("fotmob bucket: want {natural, pinned-away}, got %v", fmIDs)
	}
	if gotFM["pinned-to"] {
		t.Errorf("fotmob bucket leaked pinned-to row: %v", fmIDs)
	}

	// scores365 bucket: pinned-to only (1 total). Before the fix
	// the pinned-to row went to the fotmob bucket by accident.
	if len(s365IDs) != 1 {
		t.Fatalf("scores365 bucket: want 1 league, got %d (%v)", len(s365IDs), s365IDs)
	}
	if s365IDs[0] != "pinned-to" {
		t.Errorf("scores365 bucket: want only pinned-to, got %v", s365IDs)
	}
}
