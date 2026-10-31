package events

import (
	"strings"
	"testing"
)

func TestListPageUsesPortableLikeEscape(t *testing.T) {
	f := seedListPageFixture(t)
	recorded, rec := recordSQL(f.db)

	if ids := listPageIDs(t, recorded, EventsPageFilter{LeagueName: "Primera", TeamName: "Barcelona"}); len(ids) != 1 {
		t.Fatalf("ListPage returned %v, want event 1", ids)
	}

	stmts := rec.matching("SELECT", "LIKE")
	if len(stmts) != 1 {
		t.Fatalf("expected one events query with LIKE filters, got %v", stmts)
	}
	stmt := stmts[0]
	if got := strings.Count(stmt, "ESCAPE '!'"); got != 3 {
		t.Fatalf("want 3 LIKE clauses using ESCAPE '!' (league + both teams), got %d: %s", got, stmt)
	}
	if strings.Contains(stmt, "ESCAPE '\\'") {
		t.Fatalf("LIKE query still uses the backslash escape character: %s", stmt)
	}
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
