package scores365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// allscoresBody is a trimmed copy of a real
// /web/games/allscores/ response: one live baseball game plus the
// competition and country dictionaries needed to resolve it.
const allscoresBody = `{
  "lastUpdateId": 5758996845,
  "ttl": 5,
  "sports": [{"id":7,"name":"Béisbol","nameForURL":"baseball","imageVersion":1}],
  "countries": [{"id":323,"name":"EE.UU.","nameForURL":"usa","imageVersion":1}],
  "competitions": [{"id":438,"countryId":323,"sportId":7,"name":"MLB","nameForURL":"mlb","imageVersion":2}],
  "games": [{
    "id": 4620228,
    "sportId": 7,
    "competitionId": 438,
    "competitionDisplayName": "MLB",
    "startTime": "2026-09-13T23:20:00-06:00",
    "statusGroup": 3,
    "statusText": "6ta Entrada",
    "shortStatusText": "6",
    "gameTime": 6.0,
    "homeCompetitor": {"id":7421,"countryId":323,"sportId":7,"name":"San Francisco Giants","score":4.0,"color":"#FD5A1E","imageVersion":4},
    "awayCompetitor": {"id":7420,"countryId":323,"sportId":7,"name":"San Diego Padres","score":6.0,"color":"#2F241D","imageVersion":1}
  }]
}`

func TestClient_FetchGames_ParsesRealShape(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" {
			t.Errorf("missing Referer header")
		}
		if r.URL.Path != "/web/games/allscores/" {
			t.Errorf("path = %q, want /web/games/allscores/", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(allscoresBody))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	feed, err := c.FetchGames(context.Background(), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchGames: %v", err)
	}

	// The API takes a single day as a startDate/endDate pair in
	// DD/MM/YYYY; a real multi-day range comes back empty.
	for _, want := range []string{"startDate=14%2F09%2F2026", "endDate=14%2F09%2F2026", "appTypeId=5", "langId=29"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}

	if len(feed.Games) != 1 {
		t.Fatalf("got %d games, want 1", len(feed.Games))
	}
	g := feed.Games[0]
	if g.ID != 4620228 || g.CompetitionID != 438 || g.SportID != 7 {
		t.Errorf("game fields wrong: %+v", g)
	}
	if g.StatusGroup != StatusGroupLive {
		t.Errorf("statusGroup = %d, want %d", g.StatusGroup, StatusGroupLive)
	}
	if g.HomeCompetitor.ID != 7421 || g.HomeCompetitor.Name != "San Francisco Giants" {
		t.Errorf("home competitor wrong: %+v", g.HomeCompetitor)
	}
	if g.HomeCompetitor.Score != 4 || g.AwayCompetitor.Score != 6 {
		t.Errorf("scores wrong: %v-%v", g.HomeCompetitor.Score, g.AwayCompetitor.Score)
	}
	if len(feed.Competitions) != 1 || feed.Competitions[0].Name != "MLB" {
		t.Errorf("competitions wrong: %+v", feed.Competitions)
	}
	if len(feed.Countries) != 1 || feed.Countries[0].ID != 323 {
		t.Errorf("countries wrong: %+v", feed.Countries)
	}
}

func TestClient_FetchGames_RetryOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"games":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	if _, err := c.FetchGames(context.Background(), time.Now()); err != nil {
		t.Fatalf("FetchGames: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("hits = %d, want 3 (2 retries then success)", got)
	}
}

func TestClient_CacheTTL(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"games":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	date := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := c.FetchGames(context.Background(), date); err != nil {
			t.Fatalf("FetchGames: %v", err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (cache should hit on 2nd and 3rd call)", got)
	}
}

// The cache is keyed by request URL, so two different days must not
// share an entry — the bug that would silently make every day of
// ScrapeNext7Days return today's fixtures.
func TestClient_CacheIsPerDay(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"games":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	base := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := c.FetchGames(context.Background(), base.AddDate(0, 0, i)); err != nil {
			t.Fatalf("FetchGames: %v", err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("hits = %d, want 3 (one request per distinct day)", got)
	}
}

func TestClient_FetchCompetitions_SendsSportsFilter(t *testing.T) {
	body := `{"countries":[{"id":18,"name":"EE.UU.","nameForURL":"usa"}],
	          "competitions":[{"id":352,"countryId":18,"sportId":6,"name":"NFL","nameForURL":"nfl","imageVersion":2}]}`
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/competitions/" {
			t.Errorf("path = %q, want /web/competitions/", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	feed, err := c.FetchCompetitions(context.Background(), []int{2, 6, 7})
	if err != nil {
		t.Fatalf("FetchCompetitions: %v", err)
	}
	// Without ?sports= the endpoint only returns a short "popular"
	// list, so the filter reaching the wire is load-bearing.
	if !strings.Contains(gotQuery, "sports=2%2C6%2C7") {
		t.Errorf("query %q missing sports filter", gotQuery)
	}
	if len(feed.Competitions) != 1 || feed.Competitions[0].ID != 352 {
		t.Fatalf("competitions wrong: %+v", feed.Competitions)
	}
	if feed.Competitions[0].SportID != 6 || feed.Competitions[0].CountryID != 18 {
		t.Errorf("competition fields wrong: %+v", feed.Competitions[0])
	}
}

func TestClient_FetchCompetitions_FallsBackToConfiguredSports(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"competitions":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, SportIDs: []int{3, 4}})
	if _, err := c.FetchCompetitions(context.Background(), nil); err != nil {
		t.Fatalf("FetchCompetitions: %v", err)
	}
	if !strings.Contains(gotQuery, "sports=3%2C4") {
		t.Errorf("query %q missing configured sports filter", gotQuery)
	}
}

func TestClient_FetchSports(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/sports/" {
			t.Errorf("path = %q, want /web/sports/", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sports":[{"id":1,"name":"Fútbol","nameForURL":"football"},{"id":2,"name":"Básquetbol","nameForURL":"basketball"}]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	sports, err := c.FetchSports(context.Background())
	if err != nil {
		t.Fatalf("FetchSports: %v", err)
	}
	if len(sports) != 2 || sports[0].NameForURL != "football" || sports[1].ID != 2 {
		t.Errorf("sports wrong: %+v", sports)
	}
}

func TestClient_Options_Defaults(t *testing.T) {
	c := NewClient(Options{})
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	if c.langID != DefaultLangID {
		t.Errorf("langID = %d, want %d (es-mx)", c.langID, DefaultLangID)
	}
	if c.TimezoneName() != DefaultTimezoneName {
		t.Errorf("timezoneName = %q, want %q", c.TimezoneName(), DefaultTimezoneName)
	}
	if c.userCountryID != DefaultUserCountryID {
		t.Errorf("userCountryID = %d, want %d", c.userCountryID, DefaultUserCountryID)
	}
}

func TestImageURLs(t *testing.T) {
	c := NewClient(Options{})

	got := c.CompetitorImageURL(13693, 4)
	want := "https://imagecache.365scores.com/image/upload/f_png,w_68,h_68,c_limit,q_auto:eco,d_Competitors:default1.png/v4/Competitors/13693"
	if got != want {
		t.Errorf("CompetitorImageURL:\n got %q\nwant %q", got, want)
	}

	// An absent imageVersion must still produce a usable URL.
	if got := c.CompetitionImageURL(352, 0); !strings.Contains(got, "/v1/Competitions/352") {
		t.Errorf("CompetitionImageURL(352, 0) = %q, want v1 fallback", got)
	}
	if got := c.CountryImageURL(116, 1); !strings.Contains(got, "/Countries/116") {
		t.Errorf("CountryImageURL = %q", got)
	}
	// A zero ID has no badge; callers treat "" as nothing to fetch.
	if got := c.CompetitorImageURL(0, 1); got != "" {
		t.Errorf("CompetitorImageURL(0) = %q, want empty", got)
	}
	// The package-level helper must agree with the method so the
	// logo fallback chain in internal/events builds the same URL.
	if pkg, method := CompetitorImageURL(13693, 4), c.CompetitorImageURL(13693, 4); pkg != method {
		t.Errorf("package helper %q != client method %q", pkg, method)
	}
}

func TestClient_NonRetryableStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	if _, err := c.FetchGames(context.Background(), time.Now()); err == nil {
		t.Fatal("FetchGames: want error on 403, got nil")
	}
}
