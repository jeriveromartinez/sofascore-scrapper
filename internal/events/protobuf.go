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
		LogoUrl:        t.LogoUrl,
		Name:           t.Name,
		PrimaryColor:   t.PrimaryColor,
		SecondaryColor: t.SecondaryColor,
		TextColor:      t.TextColor,
	}
}

func EventToProto(e Event) *pb.SofaScoreEvent {
	if e.HomeTeamModel != nil {
		e.HomeTeamModel.LogoUrl = "/api/app/v1" + e.HomeTeamModel.LogoUrl
	}
	if e.AwayTeamModel != nil {
		e.AwayTeamModel.LogoUrl = "/api/app/v1" + e.AwayTeamModel.LogoUrl
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
		TeamHome:                    TeamToProto(e.HomeTeamModel),
		TeamAway:                    TeamToProto(e.AwayTeamModel),
		League:                      tournaments.TournamentPtrToProto(e.League),
	}
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
