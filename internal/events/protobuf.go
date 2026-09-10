package events

import (
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
		Name:           t.Name,
		LogoUrl:        "/api/app/v1" + TeamLogoAPIPath(t.TeamId),
		PrimaryColor:   t.PrimaryColor,
		SecondaryColor: t.SecondaryColor,
		TextColor:      t.TextColor,
	}
}

func EventToExternalProto(e Event) *pb.ExternalEvent {
	homeTeam := TeamToProto(e.HomeTeamModel)
	awayTeam := TeamToProto(e.AwayTeamModel)

	return &pb.ExternalEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   formatTime(e.CreatedAt),
		UpdatedAt:                   formatTime(e.UpdatedAt),
		ExternalMatchId:             e.ExternalMatchId,
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
		Source:                      e.Source,
	}
}

func EventsToProto(events []Event) []*pb.ExternalEvent {
	result := make([]*pb.ExternalEvent, 0, len(events))
	for _, e := range events {
		result = append(result, EventToExternalProto(e))
	}
	return result
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
