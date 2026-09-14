package scores365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scores365"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
)

const sampleFeed = `{"Games":[
{"ID":4620228,"Comp":438,"SID":7,"GT":-1,"STime":"14-09-2026 23:00","Scrs":[],"Comps":[
{"ID":7421,"Name":"Giants A","SName":"GA","CID":323,"Color":"#FD5A1E","Color2":"#000000"},
{"ID":7420,"Name":"Padres A","SName":"PA","CID":323,"Color":"#2F241D","Color2":"#FFFFFF"}
]},
{"ID":4620229,"Comp":438,"SID":7,"STID":82,"GT":-1,"STime":"14-09-2026 23:00","Scrs":[],"Comps":[
{"ID":7422,"Name":"Cubs","SName":"CUB","CID":323,"Color":"#0E3386","Color2":"#CC3433"},
{"ID":7423,"Name":"Reds","SName":"RED","CID":323,"Color":"#C6011F","Color2":"#000000"}
]}
]}`

func newFakeClient(t *testing.T) (*scores365.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleFeed))
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
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	got, err := s.DayMatches(context.Background(), date)
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
		if m.SourceMatchId != "4620228" && m.SourceMatchId != "4620229" {
			t.Errorf("unexpected match id %s", m.SourceMatchId)
		}
		if m.HomeTeam.SourceId != TeamIDPrefix+7421 && m.HomeTeam.SourceId != TeamIDPrefix+7422 {
			t.Errorf("unexpected team id %d", m.HomeTeam.SourceId)
		}
	}
}

func TestSource_DayMatches_SkipsMissingComps(t *testing.T) {
	body := `{"Games":[
{"ID":1,"Comp":438,"SID":7,"GT":-1,"STime":"14-09-2026 23:00","Scrs":[],"Comps":[{"ID":1,"Name":"A","SName":"A","CID":323,"Color":"#FFF"}]},
{"ID":2,"Comp":438,"SID":7,"GT":-1,"STime":"14-09-2026 23:00","Scrs":[],"Comps":[]}
]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := scores365.NewClient(scores365.Options{BaseURL: srv.URL})
	s := NewSource(c)
	got, err := s.DayMatches(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d matches, want 0 (only one team in Comps)", len(got))
	}
}

func TestSource_StatusMapping(t *testing.T) {
	cases := []struct {
		name          string
		gt            int
		comp          *float64
		etime         string
		stime         string
		now           time.Time
		wantStarted   bool
		wantFinished  bool
		wantCancelled bool
	}{
		{
			name:          "gt=-1 scheduled future",
			gt:            -1,
			comp:          nil,
			etime:         "",
			stime:         "14-09-2026 23:00",
			now:           time.Unix(1726000000, 0),
			wantStarted:   false,
			wantFinished:  false,
			wantCancelled: false,
		},
		{
			name:          "gt=80 live mapped",
			gt:            80,
			comp:          ptr(50.0),
			etime:         "",
			stime:         "",
			now:           time.Unix(1726000000, 0),
			wantStarted:   true,
			wantFinished:  false,
			wantCancelled: false,
		},
		{
			name:          "gt=999 unmapped future stime",
			gt:            999,
			comp:          nil,
			etime:         "",
			stime:         "14-09-2026 23:00",
			now:           time.Unix(1726000000, 0),
			wantStarted:   false,
			wantFinished:  false,
			wantCancelled: false,
		},
		{
			name:          "gt=999 unmapped past stime",
			gt:            999,
			comp:          nil,
			etime:         "",
			stime:         "14-09-2026 20:00",
			now:           time.Unix(1790000000, 0),
			wantStarted:   true,
			wantFinished:  false,
			wantCancelled: false,
		},
		{
			name:          "gt=999 unmapped completed past etime",
			gt:            999,
			comp:          ptr(100.0),
			etime:         "14-09-2026 20:00",
			stime:         "14-09-2026 18:00",
			now:           time.Unix(1790000000, 0),
			wantStarted:   true,
			wantFinished:  true,
			wantCancelled: false,
		},
		{
			name:          "gt=5 cancelled",
			gt:            5,
			comp:          nil,
			etime:         "",
			stime:         "14-09-2026 23:00",
			now:           time.Unix(1726000000, 0),
			wantStarted:   false,
			wantFinished:  false,
			wantCancelled: true,
		},
		{
			name:          "gt=3 finished",
			gt:            3,
			comp:          ptr(100.0),
			etime:         "14-09-2026 20:00",
			stime:         "14-09-2026 18:00",
			now:           time.Unix(1790000000, 0),
			wantStarted:   true,
			wantFinished:  true,
			wantCancelled: false,
		},
		{
			name:          "gt=4 postponed treated as notstarted",
			gt:            4,
			comp:          nil,
			etime:         "",
			stime:         "14-09-2026 23:00",
			now:           time.Unix(1726000000, 0),
			wantStarted:   false,
			wantFinished:  false,
			wantCancelled: false,
		},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := scores365.Game{
				ID:         1,
				GT:         c.gt,
				Completion: c.comp,
				ETime:      c.etime,
				STime:      c.stime,
			}
			var startTS time.Time
			if c.stime != "" {
				if ts, ok := parseSTime(c.stime); ok {
					startTS = ts
				}
			}
			got := statusFlags(game, startTS, c.now)
			if got.Started != c.wantStarted || got.Finished != c.wantFinished || got.Cancelled != c.wantCancelled {
				t.Errorf("case %d (%s): got (started=%v, finished=%v, cancelled=%v), want (started=%v, finished=%v, cancelled=%v)",
					i, c.name, got.Started, got.Finished, got.Cancelled,
					c.wantStarted, c.wantFinished, c.wantCancelled)
			}
		})
	}
}

func ptr(v float64) *float64 { return &v }

func TestSource_TeamFromEvent_Prefix(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	team := s.teamFromEvent(scores365.Team{ID: 7421, Name: "X", SName: "X", Color: "#FFF", Color2: "#000"})
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
}

func TestParseSTime_OK(t *testing.T) {
	ts, ok := parseSTime("14-09-2026 23:00")
	if !ok {
		t.Fatalf("parseSTime returned !ok")
	}
	want := time.Date(2026, 9, 14, 23, 0, 0, 0, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("got %v, want %v", ts, want)
	}
}

func TestParseSTime_EmptyReturnsFalse(t *testing.T) {
	if _, ok := parseSTime(""); ok {
		t.Errorf("expected !ok for empty string")
	}
}

func TestParseScrs_Baseball(t *testing.T) {
	scrs := []float64{0, 1, 1, 2, 2, 2, 3, 4}
	got := parseScrs(scrs)
	if got.home != 3 || got.away != 4 {
		t.Errorf("got %+v, want {home:3, away:4}", got)
	}
}

func TestParseScrs_Football(t *testing.T) {
	scrs := []float64{2, 1}
	got := parseScrs(scrs)
	if got.home != 2 || got.away != 1 {
		t.Errorf("got %+v, want {home:2, away:1}", got)
	}
}

func TestSource_ScheduledEvents_FiltersByLeague(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	got, err := s.ScheduledEvents(context.Background(),
		scraper.LeagueRef{Source: "scores365", SourceLeagueId: "438"}, date)
	if err != nil {
		t.Fatalf("ScheduledEvents: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2 (both matches are Comp 438)", len(got))
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

const feedWithLeagueMeta = `{"Games":[
{"ID":4620228,"Comp":438,"SID":7,"GT":-1,"STime":"14-09-2026 23:00","Scrs":[],"Comps":[
{"ID":7421,"Name":"Giants A","SName":"GA","CID":323,"Color":"#FD5A1E","Color2":"#000000"},
{"ID":7420,"Name":"Padres A","SName":"PA","CID":323,"Color":"#2F241D","Color2":"#FFFFFF"}
]}
],"Competitions":[{"ID":438,"Name":"MLB","SName":"MLB","CID":1,"Gender":0,"Type":1}],"Countries":[{"ID":1,"Name":"USA"}]}`

func newFakeClientWithFeed(t *testing.T, body string) (*scores365.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	c := scores365.NewClient(scores365.Options{BaseURL: srv.URL})
	return c, srv
}

func TestSource_DayMatches_PopulatesLeagueNameAndCountry(t *testing.T) {
	c, srv := newFakeClientWithFeed(t, feedWithLeagueMeta)
	defer srv.Close()
	s := NewSource(c)
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	got, err := s.DayMatches(context.Background(), date)
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

func TestSource_DayMatches_FallsBackOnMissingCompetition(t *testing.T) {
	c, srv := newFakeClient(t)
	defer srv.Close()
	s := NewSource(c)
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	got, err := s.DayMatches(context.Background(), date)
	if err != nil {
		t.Fatalf("DayMatches: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("got 0 matches, want at least 1")
	}
	for _, m := range got {
		if m.League.Name != "" {
			t.Errorf("League.Name = %q, want empty (no Competition in feed)", m.League.Name)
		}
		if m.League.Country != "" {
			t.Errorf("League.Country = %q, want empty (no Country in feed)", m.League.Country)
		}
	}
}
