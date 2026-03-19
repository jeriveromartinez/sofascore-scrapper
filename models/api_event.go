package models

import "fmt"

type TeamApi struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Colors struct {
		Primary   string `json:"primary"`
		Secondary string `json:"secondary"`
		Text      string `json:"text"`
	} `json:"teamColors"`
}

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
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Slug     string `json:"slug"`
			Category struct {
				Name string `json:"name"`
				Slug string `json:"slug"`
			} `json:"category"`
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

	HomeTeam TeamApi `json:"homeTeam"`

	AwayTeam TeamApi `json:"awayTeam"`

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

func (t *TeamApi) ToSofaScoreTeam() Team {
	return Team{
		TeamId:         t.ID,
		Name:           t.Name,
		PrimaryColor:   t.Colors.Primary,
		SecondaryColor: t.Colors.Secondary,
		TextColor:      t.Colors.Text,
		LogoUrl:        "https://img.sofascore.com/api/v1/team/" + fmt.Sprint(t.ID) + "/image",
	}
}

func (e *APIEvent) ToSofaScoreEvent() SofaScoreEvent {
	return SofaScoreEvent{
		SofaScoreEventId:            e.ID,
		HomeScore:                   e.HomeScore.Current,
		HomeTeamId:                  e.HomeTeam.ID,
		AwayScore:                   e.AwayScore.Current,
		AwayTeamId:                  e.AwayTeam.ID,
		StartTimestamp:              e.StartTimestamp,
		CurrentPeriodStartTimestamp: e.Time.CurrentPeriodStartTimestamp,
		Slug:                        e.Slug,
		LeagueId:                    uint(e.Tournament.UniqueTournament.ID),
	}
}
