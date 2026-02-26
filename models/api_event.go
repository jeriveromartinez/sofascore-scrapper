package models

type APIEvent struct {
	ID                              int64  `json:"id"`
	Slug                            string `json:"slug"`
	CustomID                        string `json:"customId"`
	StartTimestamp                  int64  `json:"startTimestamp"`
	WinnerCode                      *int   `json:"winnerCode"`
	AggregatedWinnerCode            *int   `json:"aggregatedWinnerCode"`
	Coverage                        int    `json:"coverage"`
	DetailID                        int    `json:"detailId"`
	PreviousLegEventID              *int64 `json:"previousLegEventId"`
	HasGlobalHighlights             bool   `json:"hasGlobalHighlights"`
	HasXg                           bool   `json:"hasXg"`
	HasEventPlayerStatistics        bool   `json:"hasEventPlayerStatistics"`
	HasEventPlayerHeatMap           bool   `json:"hasEventPlayerHeatMap"`
	CrowdsourcingDataDisplayEnabled bool   `json:"crowdsourcingDataDisplayEnabled"`
	FinalResultOnly                 bool   `json:"finalResultOnly"`
	FeedLocked                      bool   `json:"feedLocked"`
	IsEditor                        bool   `json:"isEditor"`

	Status struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Type        string `json:"type"`
	} `json:"status"`

	Tournament struct {
		ID               int64  `json:"id"`
		Name             string `json:"name"`
		UniqueTournament struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"uniqueTournament"`
	} `json:"tournament"`

	Season struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Year string `json:"year"`
	} `json:"season"`

	RoundInfo struct {
		Round int    `json:"round"`
		Name  string `json:"name"`
	} `json:"roundInfo"`

	HomeTeam struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"homeTeam"`

	AwayTeam struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"awayTeam"`

	HomeScore struct {
		Current int `json:"current"`
		Display int `json:"display"`
	} `json:"homeScore"`

	AwayScore struct {
		Current int `json:"current"`
		Display int `json:"display"`
	} `json:"awayScore"`

	Time struct {
		CurrentPeriodStartTimestamp int64 `json:"currentPeriodStartTimestamp"`
	} `json:"time"`
}

func (e APIEvent) ToSofaScoreEvent() SofaScoreEvent {
	return SofaScoreEvent{
		SofaScoreEventId:            e.ID,
		HomeTeam:                    e.HomeTeam.Name,
		HomeScore:                   e.HomeScore.Current,
		HomeTeamId:                  e.HomeTeam.ID,
		AwayTeam:                    e.AwayTeam.Name,
		AwayScore:                   e.AwayScore.Current,
		AwayTeamId:                  e.AwayTeam.ID,
		StartTimestamp:              e.StartTimestamp,
		CurrentPeriodStartTimestamp: e.Time.CurrentPeriodStartTimestamp,
		Slug:                        e.Slug,
		LeagueName:                  e.Tournament.Name,
	}
}
