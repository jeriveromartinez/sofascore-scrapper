package fotmob

type apiMatch struct {
	Id         string       `json:"id"`
	Slug       string       `json:"slug"`
	Home       apiTeam      `json:"home"`
	Away       apiTeam      `json:"away"`
	Status     apiStatus    `json:"status"`
	Time       apiTime      `json:"time"`
	League     apiLeagueRef `json:"league"`
	Tournament apiLeagueRef `json:"tournament"`
}

type apiTeam struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	ImageUrl       string `json:"imageUrl"`
	PrimaryColor   string `json:"primaryColor"`
	SecondaryColor string `json:"secondaryColor"`
	TextColor      string `json:"textColor"`
}

type apiStatus struct {
	Code      int    `json:"code"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Finished  bool   `json:"finished"`
	Started   bool   `json:"started"`
	Cancelled bool   `json:"cancelled"`
	ScoreStr  string `json:"scoreStr"`
}

type apiTime struct {
	UtcTime                     string `json:"utcTime"`
	CurrentPeriodStartTimestamp int64  `json:"currentPeriodStartTimestamp"`
}

type apiLeagueRef struct {
	Id      int64  `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	Sport   string `json:"sport"`
}

type apiMatchesResponse struct {
	Matches  struct {
		AllMatches []apiMatch `json:"allMatches"`
	} `json:"matches"`
	LeagueId int64 `json:"leagueId"`
}
