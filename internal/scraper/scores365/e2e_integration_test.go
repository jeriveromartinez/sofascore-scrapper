//go:build integration

package scores365_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	upstream "github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
	source365 "github.com/jeriveromartinez/scores365-scrapper/internal/scraper/scores365"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

// TestScores365EndToEnd_PersistsDayFixture walks the whole 365scores
// path in one test: a real-shape /web/games/allscores/ payload served
// over HTTP, through the client, the Source adapter, the domain
// conversion, and finally the repository upsert into SQLite. Every
// other test in this package starts after the HTTP layer, so a break
// in the wiring between those layers would not fail anything.
func TestScores365EndToEnd_PersistsDayFixture(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	client := upstream.NewClient(upstream.Options{
		BaseURL:  srv.URL,
		CacheTTL: time.Nanosecond,
	})
	source := source365.NewSource(client)

	matches, err := source.DayMatches(context.Background(), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3 (two football, one baseball)", len(matches))
	}

	db := setupE2ETestDB(t)
	repo := events.NewRepository(db)

	// ToScrapeBatch takes a single sport for the whole batch, but a
	// day spans every configured sport. Group before converting, the
	// same way the dispatcher does.
	for _, group := range groupBySport(matches) {
		batch, err := scraper.ToScrapeBatch(group.matches, group.sport)
		if err != nil {
			t.Fatalf("ToScrapeBatch(%s): %v", group.sport, err)
		}
		if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
			t.Fatalf("UpsertScrapeBatch(%s): %v", group.sport, err)
		}
	}

	// The baseball game is the interesting one: it exercises score
	// extraction, the in-progress status mapping and the league
	// dictionary lookup in a single row.
	var got events.Event
	err = db.Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League").
		First(&got, "external_match_id = ?", "scores365-4620229").Error
	if err != nil {
		t.Fatalf("load baseball event: %v", err)
	}

	if got.Source != "scores365" {
		t.Errorf("source = %q, want scores365", got.Source)
	}
	if got.Sport != "baseball" {
		t.Errorf("sport = %q, want baseball", got.Sport)
	}
	if got.StatusType != "inprogress" {
		t.Errorf("status_type = %q, want inprogress", got.StatusType)
	}
	if got.HomeScore != 4 || got.AwayScore != 6 {
		t.Errorf("score = %d-%d, want 4-6", got.HomeScore, got.AwayScore)
	}

	wantStart := time.Date(2026, 9, 15, 5, 20, 0, 0, time.UTC).UnixMilli()
	if got.StartTimestamp != wantStart {
		t.Errorf("start_timestamp = %d, want %d", got.StartTimestamp, wantStart)
	}

	// Source IDs are namespaced so 365scores rows cannot collide with
	// FotMob ones in the shared teams table.
	if got.HomeTeamId != source365.TeamIDPrefix+7430 {
		t.Errorf("home team id = %d, want %d", got.HomeTeamId, source365.TeamIDPrefix+7430)
	}
	if got.AwayTeamId != source365.TeamIDPrefix+7431 {
		t.Errorf("away team id = %d, want %d", got.AwayTeamId, source365.TeamIDPrefix+7431)
	}
	if got.LeagueId != uint(upstream.LeagueIDPrefix+439) {
		t.Errorf("league id = %d, want %d", got.LeagueId, upstream.LeagueIDPrefix+439)
	}

	if got.HomeTeamModel == nil || got.HomeTeamModel.Name != "San Francisco Giants" {
		t.Errorf("home team = %#v, want San Francisco Giants", got.HomeTeamModel)
	}
	if got.AwayTeamModel == nil || got.AwayTeamModel.Name != "San Diego Padres" {
		t.Errorf("away team = %#v, want San Diego Padres", got.AwayTeamModel)
	}
	if got.League == nil || got.League.Name != "MLB" || got.League.Region != "EE.UU." {
		t.Errorf("league = %#v, want MLB / EE.UU.", got.League)
	}

	// prepareTeamLogo rewrites the CDN URL to a local API path before
	// insert; the scheduler downloads from the original.
	if got.HomeTeamModel == nil || got.HomeTeamModel.LogoUrl != events.TeamLogoAPIPath(got.HomeTeamId) {
		t.Errorf("home logo = %q, want local path %q", got.HomeTeamModel.LogoUrl, events.TeamLogoAPIPath(got.HomeTeamId))
	}

	// A scheduled fixture reports -1 on both sides. Those zero out via
	// the scheduled branch of scorePair, before the clamp runs.
	var scheduled events.Event
	if err := db.First(&scheduled, "external_match_id = ?", "scores365-4620228").Error; err != nil {
		t.Fatalf("load scheduled event: %v", err)
	}
	if scheduled.Sport != "football" {
		t.Errorf("sport = %q, want football", scheduled.Sport)
	}
	if scheduled.StatusType != "notstarted" {
		t.Errorf("status_type = %q, want notstarted", scheduled.StatusType)
	}
	if scheduled.HomeScore != 0 || scheduled.AwayScore != 0 {
		t.Errorf("score = %d-%d, want 0-0", scheduled.HomeScore, scheduled.AwayScore)
	}
	if scheduled.LeagueId != uint(upstream.LeagueIDPrefix+438) {
		t.Errorf("league id = %d, want %d", scheduled.LeagueId, upstream.LeagueIDPrefix+438)
	}

	// A live fixture at 0-0 reports -1 as well, but reaches the score
	// through clampScore instead. Same stored value, different path.
	var goalless events.Event
	if err := db.First(&goalless, "external_match_id = ?", "scores365-4620230").Error; err != nil {
		t.Fatalf("load goalless event: %v", err)
	}
	if goalless.StatusType != "inprogress" {
		t.Errorf("status_type = %q, want inprogress", goalless.StatusType)
	}
	if goalless.HomeScore != 0 || goalless.AwayScore != 0 {
		t.Errorf("score = %d-%d, want 0-0 (the -1 sentinel must not leak)", goalless.HomeScore, goalless.AwayScore)
	}

	var teamCount, tournamentCount, eventCount int64
	db.Model(&events.Team{}).Count(&teamCount)
	db.Model(&tournaments.Tournament{}).Count(&tournamentCount)
	db.Model(&events.Event{}).Count(&eventCount)

	if teamCount != 6 {
		t.Errorf("teams = %d, want 6", teamCount)
	}
	if tournamentCount != 2 {
		t.Errorf("tournaments = %d, want 2", tournamentCount)
	}
	if eventCount != 3 {
		t.Errorf("events = %d, want 3", eventCount)
	}
}

type sportGroup struct {
	sport   string
	matches []scraper.Match
}

func groupBySport(matches []scraper.Match) []sportGroup {
	bySport := make(map[string][]scraper.Match)
	order := make([]string, 0, 2)
	for _, m := range matches {
		if _, seen := bySport[m.League.Sport]; !seen {
			order = append(order, m.League.Sport)
		}
		bySport[m.League.Sport] = append(bySport[m.League.Sport], m)
	}
	groups := make([]sportGroup, 0, len(order))
	for _, sport := range order {
		groups = append(groups, sportGroup{sport: sport, matches: bySport[sport]})
	}
	return groups
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile("testdata/allscores_day.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// Validate at load time so a malformed fixture fails the test with
	// a fixture error rather than a confusing decode failure later.
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/games/allscores/" {
			t.Errorf("request path = %q, want /web/games/allscores/", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("startDate"); got != "14/09/2026" {
			t.Errorf("startDate = %q, want 14/09/2026", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write fixture: %v", err)
		}
	}))
}

func setupE2ETestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&events.Event{}, &events.Team{}, &tournaments.Tournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
