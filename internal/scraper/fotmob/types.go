package fotmob

// PR #124: the wire shape below matches the actual
// `https://www.fotmob.com/api/data/matches?date=YYYYMMDD&timezone=…`
// endpoint (verified with curl on 2026-09-11). The previous types
// modelled the legacy `/api/leagues` endpoint (envelope
// `{matches:{allMatches:[]}}`, string match IDs, color fields on
// every team) — none of which match the real response anymore.

// apiMatchesResponse is the top-level shape FotMob returns for
// /api/data/matches on a given date. Matches are grouped per
// league; the client filters by league ID client-side because the
// endpoint has no per-league filter at the URL level.
type apiMatchesResponse struct {
	Leagues []apiLeagueGroup `json:"leagues"`
	Date    string           `json:"date"`
}

// apiLeagueGroup is one FotMob league for the day. `Id` and
// `Matches` are the fields the scraper cares about; everything else
// is captured for parity with the upstream payload so future
// refactors don't have to re-extend the struct.
type apiLeagueGroup struct {
	Id           int64      `json:"id"`
	PrimaryId    int64      `json:"primaryId"`
	Ccode        string     `json:"ccode"`
	Name         string     `json:"name"`
	Matches      []apiMatch `json:"matches"`
	InternalRank int        `json:"internalRank"`
	SimpleLeague bool       `json:"simpleLeague"`
	LocalRank    int        `json:"localRank"`
}

// apiMatch is one FotMob match. The previous version modelled `Id`
// as a string and lacked the top-level `StatusId` / `Score` fields
// on teams; both are corrected here.
type apiMatch struct {
	Id               int64     `json:"id"`
	LeagueId         int64     `json:"leagueId"`
	Time             string    `json:"time"`
	Home             apiTeam   `json:"home"`
	Away             apiTeam   `json:"away"`
	EliminatedTeamId *int64    `json:"eliminatedTeamId"`
	StatusId         int       `json:"statusId"`
	TournamentStage  string    `json:"tournamentStage"`
	Status           apiStatus `json:"status"`
	TimeTS           int64     `json:"timeTS"`
}

// apiTeam is one FotMob team within a match. The legacy schema
// carried ImageUrl/PrimaryColor/SecondaryColor/TextColor; the
// current /api/data/matches payload exposes only id/score/name and
// the short/long display variants. Score is the source of truth for
// the match result — Status.ScoreStr (e.g. "1 - 3" with whitespace)
// is kept available on apiStatus but the scraper reads scores from
// these int fields directly.
type apiTeam struct {
	Id        int64  `json:"id"`
	Score     int    `json:"score"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
}

// apiStatus is the nested status block on each match. The `Reason`
// field is a free-form object ({"short","long",…}); we keep it as
// map[string]any because FotMob adds new reason keys over time and
// we only read `short` for the human label.
type apiStatus struct {
	UtcTime      string         `json:"utcTime"`
	Halfs        map[string]any `json:"halfs"`
	PeriodLength int            `json:"periodLength"`
	Finished     bool           `json:"finished"`
	Started      bool           `json:"started"`
	Cancelled    bool           `json:"cancelled"`
	Awarded      bool           `json:"awarded"`
	ScoreStr     string         `json:"scoreStr"`
	Reason       map[string]any `json:"reason"`
}

// apiSuggestResponse models FotMob's /api/searchapi/suggest payload.
// The endpoint returns a heterogeneous list of typed entries
// (league/team/player/manager). Only the league-typed entries are
// relevant to the catalog seeder; the rest are filtered out by
// Source.SearchLeagues.
type apiSuggestResponse struct {
	Suggestions []apiSuggestEntry `json:"suggestions"`
}

type apiSuggestEntry struct {
	Type    string `json:"type"`
	Id      int64  `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	Sport   string `json:"sport"`
}
