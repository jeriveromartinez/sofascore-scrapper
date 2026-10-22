package events

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

type listPageFixture struct {
	db     *gorm.DB
	league tournaments.Tournament
	home   Team
	away   Team
}

func seedListPageFixture(t *testing.T) listPageFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	f := listPageFixture{
		db:     db,
		league: tournaments.Tournament{Name: "Primera Division", Slug: "primera"},
		home:   Team{TeamId: 5001, Name: "Barcelona SC"},
		away:   Team{TeamId: 5002, Name: "Emelec"},
	}
	for _, row := range []any{&f.league, &f.home, &f.away} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := db.Create(&Event{
		ExternalMatchId: "1", Sport: "football", StartTimestamp: 1000,
		LeagueId: f.league.ID, HomeTeamId: f.home.TeamId, AwayTeamId: f.away.TeamId,
	}).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return f
}

func listPageIDs(t *testing.T, db *gorm.DB, filter EventsPageFilter) []string {
	t.Helper()
	filter.Limit = 10
	rows, _, err := NewRepository(db).ListPage(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ExternalMatchId)
	}
	return ids
}

func TestListPageJoinsOnlyForNameFilters(t *testing.T) {
	f := seedListPageFixture(t)
	recorded, rec := recordSQL(f.db)

	cases := []struct {
		name       string
		filter     EventsPageFilter
		wantJoined []string
	}{
		{name: "no name filter", filter: EventsPageFilter{Sport: "football"}},
		{name: "league filter", filter: EventsPageFilter{LeagueName: "Primera"}, wantJoined: []string{"leagues"}},
		{name: "team filter", filter: EventsPageFilter{TeamName: "Emelec"}, wantJoined: []string{"home_team_models", "away_team_models"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec.reset()
			if ids := listPageIDs(t, recorded, tc.filter); len(ids) != 1 {
				t.Fatalf("ListPage returned %v, want event 1", ids)
			}
			mains := rec.matching("SELECT", "FROM `events`")
			if len(mains) != 1 {
				t.Fatalf("expected one events query, got %v", mains)
			}
			joins := strings.Count(strings.ToUpper(mains[0]), "JOIN")
			if joins != len(tc.wantJoined) {
				t.Fatalf("events query has %d JOINs, want %d: %s", joins, len(tc.wantJoined), mains[0])
			}
			for _, alias := range tc.wantJoined {
				if !strings.Contains(mains[0], alias) {
					t.Fatalf("events query missing join %q: %s", alias, mains[0])
				}
			}
		})
	}
}

func TestListPageNameFiltersIgnoreSoftDeletedRows(t *testing.T) {
	f := seedListPageFixture(t)

	if ids := listPageIDs(t, f.db, EventsPageFilter{TeamName: "Barcelona"}); len(ids) != 1 {
		t.Fatalf("team filter before delete = %v, want [1]", ids)
	}
	if ids := listPageIDs(t, f.db, EventsPageFilter{LeagueName: "Primera"}); len(ids) != 1 {
		t.Fatalf("league filter before delete = %v, want [1]", ids)
	}

	if err := f.db.Delete(&f.home).Error; err != nil {
		t.Fatalf("soft delete team: %v", err)
	}
	if err := f.db.Delete(&f.league).Error; err != nil {
		t.Fatalf("soft delete league: %v", err)
	}

	if ids := listPageIDs(t, f.db, EventsPageFilter{TeamName: "Barcelona"}); len(ids) != 0 {
		t.Fatalf("deleted team name still matches: %v", ids)
	}
	if ids := listPageIDs(t, f.db, EventsPageFilter{LeagueName: "Primera"}); len(ids) != 0 {
		t.Fatalf("deleted league name still matches: %v", ids)
	}
	if ids := listPageIDs(t, f.db, EventsPageFilter{TeamName: "Emelec"}); len(ids) != 1 {
		t.Fatalf("live away team must still match: %v", ids)
	}
}
