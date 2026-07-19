package events

import (
	"strings"
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
)

func TeamToProto(t *Team) *pb.Team {
	if t == nil {
		return nil
	}
	return &pb.Team{
		Id:             uint32(t.ID),
		TeamId:         t.TeamId,
		LogoUrl:        t.LogoUrl,
		Name:           t.Name,
		PrimaryColor:   t.PrimaryColor,
		SecondaryColor: t.SecondaryColor,
		TextColor:      t.TextColor,
	}
}

func EventToProto(e Event) *pb.SofaScoreEvent {
	homeTeam := TeamToProto(e.HomeTeamModel)
	if homeTeam != nil {
		homeTeam.LogoUrl = logoURLForAPI(homeTeam.LogoUrl)
	}
	awayTeam := TeamToProto(e.AwayTeamModel)
	if awayTeam != nil {
		awayTeam.LogoUrl = logoURLForAPI(awayTeam.LogoUrl)
	}

	return &pb.SofaScoreEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   formatTime(e.CreatedAt),
		UpdatedAt:                   formatTime(e.UpdatedAt),
		SofaScoreEventId:            e.SofaScoreEventId,
		Sport:                       e.Sport,
		HomeScore:                   int32(e.HomeScore),
		HomeTeamId:                  e.HomeTeamId,
		AwayScore:                   int32(e.AwayScore),
		AwayTeamId:                  e.AwayTeamId,
		ScrapedAt:                   e.ScrapedAt,
		StartTimestamp:              e.StartTimestamp,
		CurrentPeriodStartTimestamp: e.CurrentPeriodStartTimestamp,
		Slug:                        e.Slug,
		StatusType:                  e.StatusType,
		TeamHome:                    homeTeam,
		TeamAway:                    awayTeam,
		League:                      tournaments.TournamentPtrToProto(e.League),
	}
}

func logoURLForAPI(rawURL string) string {
	const apiPrefix = "/api/app/v1"
	if strings.HasPrefix(rawURL, "/") && rawURL != apiPrefix && !strings.HasPrefix(rawURL, apiPrefix+"/") {
		return apiPrefix + rawURL
	}
	return rawURL
}

func EventsToProto(events []Event) []*pb.SofaScoreEvent {
	result := make([]*pb.SofaScoreEvent, 0, len(events))
	for _, e := range events {
		result = append(result, EventToProto(e))
	}
	return result
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
