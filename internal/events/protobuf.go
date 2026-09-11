package events

import (
	"strconv"
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

	// Backwards-compat shim for pre-FotMob clients (see fix A2,
	// PR #122). When the stored ExternalMatchId is numeric we echo it
	// into the deprecated int64 field so clients built against the
	// old schema keep seeing the legacy sofa_score_event_id at tag 4.
	// Non-numeric ids (e.g. "fotmob-abc-42") leave the deprecated
	// field at the proto zero value: the int64 cannot represent them
	// and new clients read the string field at tag 20.
	var deprecatedID int64
	if n, err := strconv.ParseInt(e.ExternalMatchId, 10, 64); err == nil {
		deprecatedID = n
	}

	return &pb.ExternalEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   formatTime(e.CreatedAt),
		UpdatedAt:                   formatTime(e.UpdatedAt),
		SofaScoreEventIdDeprecated:  deprecatedID,
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
		ExternalMatchId:             e.ExternalMatchId,
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
