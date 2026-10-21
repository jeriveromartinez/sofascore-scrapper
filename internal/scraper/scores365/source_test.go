package scores365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
)

// sampleFeed mirrors the /web/games/allscores/ shape: two scheduled
// baseball fixtures with the competitors embedded on each game, and
// no competition/country dictionary (the fallback path).
const sampleFeed = `{"games":[
{"id":4620228,"sportId":7,"competitionId":438,"startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,"statusText":"Programado",
 "homeCompetitor":{"id":7421,"name":"Giants A","countryId":323,"color":"#FD5A1E","awayColor":"#000000","score":-1,"imageVersion":4},
 "awayCompetitor":{"id":7420,"name":"Padres A","countryId":323,"color":"#2F241D","awayColor":"#FFFFFF","score":-1,"imageVersion":1}},
{"id":4620229,"sportId":7,"competitionId":438,"startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,"statusText":"Programado",
 "homeCompetitor":{"id":7422,"name":"Cubs","countryId":323,"color":"#0E3386","awayColor":"#CC3433","score":-1,"imageVersion":1},
 "awayCompetitor":{"id":7423,"name":"Reds","countryId":323,"color":"#C6011F","awayColor":"#000000","score":-1,"imageVersion":1}}
]}`

func newFakeClient(t *testing.T) (*scores365.Client, *httptest.Server) {
	t.Helper()
	return newFakeClientWithFeed(t, sampleFeed)
}

func newFakeClientWithFeed(t *testing.T, body string) (*scores365.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	c := scores365.NewClient(scores365.Options{BaseURL: srv.URL})
	return c, srv
}

func TestSource_Name(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	if s.Name() != "scores365" {
		t.Errorf("Name() = %q, want scores365", s.Name())
	}
}

func TestSource_DayMatches_AllSports(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	got, err := s.DayMatches(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2", len(got))
	}
	for _, m := range got {
		if m.Source != "scores365" {
			t.Errorf("match source = %q, want scores365", m.Source)
		}
		// SourceMatchId must be prefixed with "scores365-" so the
		// events.external_match_id global unique index never collides
		// with a FotMob row (FotMob IDs are raw decimals, e.g. "4193492").
		if m.SourceMatchId != "scores365-4620228" && m.SourceMatchId != "scores365-4620229" {
			t.Errorf("unexpected match id %s (want scores365- prefixed)", m.SourceMatchId)
		}
		// competitionId 438 -> 6000000438, so the 365scores range
		// cannot collide with FotMob league IDs on the tournaments table.
		if m.League.SourceLeagueId != "6000000438" {
			t.Errorf("unexpected league id %s (want 6000000438)", m.League.SourceLeagueId)
		}
		if m.League.Sport != "baseball" {
			t.Errorf("League.Sport = %q, want baseball (sportId 7)", m.League.Sport)
		}
		if m.HomeTeam.SourceId != TeamIDPrefix+7421 && m.HomeTeam.SourceId != TeamIDPrefix+7422 {
			t.Errorf("unexpected team id %d", m.HomeTeam.SourceId)
		}
		// A scheduled fixture carries score -1 on the wire; it must not
		// reach the API as a negative.
		if m.HomeScore != 0 || m.AwayScore != 0 {
			t.Errorf("scheduled fixture scores = %d-%d, want 0-0", m.HomeScore, m.AwayScore)
		}
		if m.Status.Started || m.Status.Finished || m.Status.Cancelled {
			t.Errorf("statusGroup 2 should be notstarted, got %+v", m.Status)
		}
		if m.StartTimestamp.IsZero() {
			t.Errorf("StartTimestamp not parsed")
		}
	}
}

func TestSource_DayMatches_SkipsIncompleteGames(t *testing.T) {
	// Games missing a competitor, a competition or a parseable
	// startTime cannot produce a usable row and must be dropped.
	body := `{"games":[
{"id":1,"sportId":7,"competitionId":438,"startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,
 "homeCompetitor":{"id":1,"name":"A"},"awayCompetitor":{"id":0,"name":""}},
{"id":2,"sportId":7,"competitionId":0,"startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,
 "homeCompetitor":{"id":3,"name":"C"},"awayCompetitor":{"id":4,"name":"D"}},
{"id":3,"sportId":7,"competitionId":438,"startTime":"","statusGroup":2,
 "homeCompetitor":{"id":5,"name":"E"},"awayCompetitor":{"id":6,"name":"F"}}
]}`
	c, srv := newFakeClientWithFeed(t, body)
	defer srv.Close()
	s := NewSource(c)
	got, err := s.DayMatches(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d matches, want 0 (all three games are incomplete)", len(got))
	}
}

// The legacy /data/games feed only served the current UTC day, so
// DayMatches short-circuited every other date and ScrapeNext7Days got
// real data for exactly one of its seven iterations. The web API takes
// an explicit startDate/endDate pair, so every date must now issue its
// own request.
func TestSource_DayMatches_FetchesNonTodayDates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if !strings.Contains(r.URL.RawQuery, "startDate=") {
			t.Errorf("request missing startDate: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()
	c := scores365.NewClient(scores365.Options{BaseURL: srv.URL})
	s := NewSource(c)

	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	got, err := s.DayMatches(context.Background(), tomorrow)
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d matches for tomorrow, want 2", len(got))
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("hits = %d, want 1 (a non-today date must hit the API)", hits)
	}
}

func TestSource_StatusMapping(t *testing.T) {
	cases := []struct {
		name          string
		statusGroup   int
		statusText    string
		wantStarted   bool
		wantFinished  bool
		wantCancelled bool
		wantType      string
	}{
		{
			name:        "group 2 scheduled",
			statusGroup: scores365.StatusGroupScheduled,
			statusText:  "Programado",
			wantType:    "NS",
		},
		{
			name:        "group 3 live",
			statusGroup: scores365.StatusGroupLive,
			statusText:  "6ta Entrada",
			wantStarted: true,
			wantType:    "LIVE",
		},
		{
			name:         "group 4 ended",
			statusGroup:  scores365.StatusGroupEnded,
			statusText:   "Finalizado",
			wantStarted:  true,
			wantFinished: true,
			wantType:     "FT",
		},
		{
			// The modern payload dropped the numeric GT code, so a
			// cancelled fixture is only distinguishable by its
			// localized status text.
			name:          "ended group with cancelled text",
			statusGroup:   scores365.StatusGroupEnded,
			statusText:    "Cancelado",
			wantCancelled: true,
			wantType:      "CANC",
		},
		{
			name:          "scheduled group with postponed text",
			statusGroup:   scores365.StatusGroupScheduled,
			statusText:    "Aplazado",
			wantCancelled: true,
			wantType:      "CANC",
		},
		{
			name:          "english postponed",
			statusGroup:   scores365.StatusGroupScheduled,
			statusText:    "Postponed",
			wantCancelled: true,
			wantType:      "CANC",
		},
		{
			name:          "suspended",
			statusGroup:   scores365.StatusGroupEnded,
			statusText:    "Suspendido",
			wantCancelled: true,
			wantType:      "CANC",
		},
		{
			// A tennis retirement is a finished match, not a
			// cancelled one — the keyword table deliberately
			// excludes "retirado"/"retired".
			name:         "retired is finished",
			statusGroup:  scores365.StatusGroupEnded,
			statusText:   "Retirado",
			wantStarted:  true,
			wantFinished: true,
			wantType:     "FT",
		},
		{
			// An unrecognized group degrades to notstarted rather
			// than silently marking the fixture live.
			name:        "unknown group defaults to scheduled",
			statusGroup: 99,
			statusText:  "???",
			wantType:    "NS",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := scores365.Game{ID: 1, StatusGroup: c.statusGroup, StatusText: c.statusText}
			got := statusFlags(game, classify(game))
			if got.Started != c.wantStarted || got.Finished != c.wantFinished || got.Cancelled != c.wantCancelled {
				t.Errorf("got (started=%v, finished=%v, cancelled=%v), want (started=%v, finished=%v, cancelled=%v)",
					got.Started, got.Finished, got.Cancelled,
					c.wantStarted, c.wantFinished, c.wantCancelled)
			}
			if got.Type != c.wantType {
				t.Errorf("Type = %q, want %q", got.Type, c.wantType)
			}
			if got.Code != c.statusGroup {
				t.Errorf("Code = %d, want %d (upstream statusGroup)", got.Code, c.statusGroup)
			}
			if got.Description != c.statusText {
				t.Errorf("Description = %q, want %q (upstream statusText)", got.Description, c.statusText)
			}
		})
	}
}

func TestSource_TeamFromCompetitor(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	team := s.teamFromCompetitor(scores365.Competitor{
		ID: 7421, Name: "X", Color: "#FFF", AwayColor: "#000", ImageVersion: 4,
	})
	if team.SourceId != TeamIDPrefix+7421 {
		t.Errorf("team.SourceId = %d, want %d", team.SourceId, TeamIDPrefix+7421)
	}
	if team.PrimaryColor != "#FFF" {
		t.Errorf("team.PrimaryColor = %q, want #FFF", team.PrimaryColor)
	}
	if team.SecondaryColor != "#000" {
		t.Errorf("team.SecondaryColor = %q, want #000", team.SecondaryColor)
	}
	if team.Name != "X" {
		t.Errorf("team.Name = %q, want X", team.Name)
	}
	// The badge comes from the 365scores CDN keyed by the team's
	// natural (unprefixed) ID plus its imageVersion cache-buster.
	if !strings.HasSuffix(team.LogoURL, "/v4/Competitors/7421") {
		t.Errorf("team.LogoURL = %q, want a v4 Competitors/7421 URL", team.LogoURL)
	}
}

func TestParseStartTime(t *testing.T) {
	ts, ok := parseStartTime("2026-09-14T23:00:00-06:00")
	if !ok {
		t.Fatalf("parseStartTime returned !ok")
	}
	// The API stamps the offset of the requested timezoneName; the
	// instant is normalized to UTC because that is what events stores.
	want := time.Date(2026, 9, 15, 5, 0, 0, 0, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("got %v, want %v", ts, want)
	}
	if ts.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", ts.Location())
	}
}

func TestParseStartTime_Invalid(t *testing.T) {
	for _, in := range []string{"", "14-09-2026 23:00", "not-a-time"} {
		if _, ok := parseStartTime(in); ok {
			t.Errorf("parseStartTime(%q) = ok, want !ok", in)
		}
	}
}

func TestScorePair(t *testing.T) {
	live := scores365.Game{
		StatusGroup:    scores365.StatusGroupLive,
		HomeCompetitor: scores365.Competitor{Score: 3},
		AwayCompetitor: scores365.Competitor{Score: 4},
	}
	if h, a := scorePair(live, statusLive); h != 3 || a != 4 {
		t.Errorf("live scorePair = %d-%d, want 3-4", h, a)
	}

	// -1 is the "no score yet" sentinel; it must never reach the API.
	sentinel := scores365.Game{
		StatusGroup:    scores365.StatusGroupEnded,
		HomeCompetitor: scores365.Competitor{Score: scores365.ScoreNone},
		AwayCompetitor: scores365.Competitor{Score: scores365.ScoreNone},
	}
	if h, a := scorePair(sentinel, statusFinished); h != 0 || a != 0 {
		t.Errorf("sentinel scorePair = %d-%d, want 0-0", h, a)
	}

	scheduled := scores365.Game{
		HomeCompetitor: scores365.Competitor{Score: 7},
		AwayCompetitor: scores365.Competitor{Score: 9},
	}
	if h, a := scorePair(scheduled, statusScheduled); h != 0 || a != 0 {
		t.Errorf("scheduled scorePair = %d-%d, want 0-0", h, a)
	}
}

func TestSportSlugMapping(t *testing.T) {
	if got := SportSlug(7); got != "baseball" {
		t.Errorf("SportSlug(7) = %q, want baseball", got)
	}
	if got := SportSlug(1234); got != "unknown" {
		t.Errorf("SportSlug(1234) = %q, want unknown", got)
	}
	if got := SportIDForSlug("ice-hockey"); got != 4 {
		t.Errorf("SportIDForSlug(ice-hockey) = %d, want 4", got)
	}
	// The catalog and the admin UI use a few alternate spellings.
	if got := SportIDForSlug("hockey"); got != 4 {
		t.Errorf("SportIDForSlug(hockey) = %d, want 4", got)
	}
	if got := SportIDForSlug("a.-football"); got != 6 {
		t.Errorf("SportIDForSlug(a.-football) = %d, want 6", got)
	}
	if got := SportIDForSlug("quidditch"); got != 0 {
		t.Errorf("SportIDForSlug(quidditch) = %d, want 0", got)
	}
}

func TestSource_ScheduledEvents_FiltersByLeague(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	got, err := s.ScheduledEvents(context.Background(),
		scraper.LeagueRef{Source: "scores365", SourceLeagueId: "6000000438"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("ScheduledEvents: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2 (both games are competitionId 438 -> 6000000438)", len(got))
	}

	none, err := s.ScheduledEvents(context.Background(),
		scraper.LeagueRef{Source: "scores365", SourceLeagueId: "6000000999"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("ScheduledEvents: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("got %d for an unrelated league, want 0", len(none))
	}
}

func TestSource_SearchLeagues_NotSupported(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	results, err := s.SearchLeagues(context.Background(), "nba")
	if err == nil {
		t.Errorf("expected error from SearchLeagues, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

const feedWithLeagueMeta = `{
 "countries":[{"id":1,"name":"USA"}],
 "competitions":[{"id":438,"countryId":1,"sportId":7,"name":"MLB","nameForURL":"mlb"}],
 "games":[
{"id":4620228,"sportId":7,"competitionId":438,"competitionDisplayName":"MLB - Zona B",
 "startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,
 "homeCompetitor":{"id":7421,"name":"Giants A","score":-1},
 "awayCompetitor":{"id":7420,"name":"Padres A","score":-1}}
]}`

func TestSource_DayMatches_PopulatesLeagueNameAndCountry(t *testing.T) {
	c, srv := newFakeClientWithFeed(t, feedWithLeagueMeta)
	defer srv.Close()
	s := NewSource(c)
	got, err := s.DayMatches(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	m := got[0]
	if m.League.Name != "MLB" {
		t.Errorf("League.Name = %q, want MLB", m.League.Name)
	}
	if m.League.Country != "USA" {
		t.Errorf("League.Country = %q, want USA", m.League.Country)
	}
}

// When the competition dictionary does not carry the game's
// competition, the per-game display name is the only label available.
func TestSource_DayMatches_FallsBackToDisplayName(t *testing.T) {
	body := `{"games":[
{"id":1,"sportId":7,"competitionId":438,"competitionDisplayName":"Primera C - Zona B",
 "startTime":"2026-09-14T23:00:00+00:00","statusGroup":2,
 "homeCompetitor":{"id":1,"name":"A","score":-1},
 "awayCompetitor":{"id":2,"name":"B","score":-1}}
]}`
	c, srv := newFakeClientWithFeed(t, body)
	defer srv.Close()
	s := NewSource(c)
	got, err := s.DayMatches(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if got[0].League.Name != "Primera C - Zona B" {
		t.Errorf("League.Name = %q, want the game's display name", got[0].League.Name)
	}
	if got[0].League.Country != "" {
		t.Errorf("League.Country = %q, want empty (no country dictionary)", got[0].League.Country)
	}
}

// The keyword table is written unaccented and matched against a
// folded, lowercased copy of the upstream label.
func TestHasAbandonedKeyword(t *testing.T) {
	positives := []string{"Cancelado", "APLAZADO", "Partido suspendido por lluvia", "Called off", "abandoned"}
	for _, in := range positives {
		if !hasAbandonedKeyword(in) {
			t.Errorf("hasAbandonedKeyword(%q) = false, want true", in)
		}
	}
	negatives := []string{"", "Finalizado", "Programado", "6ta Entrada", "Retirado", "Medio tiempo"}
	for _, in := range negatives {
		if hasAbandonedKeyword(in) {
			t.Errorf("hasAbandonedKeyword(%q) = true, want false", in)
		}
	}
	// foldAccents runs on an already-lowercased string.
	if got := foldAccents("sesión única"); got != "sesion unica" {
		t.Errorf("foldAccents = %q, want %q", got, "sesion unica")
	}
}
