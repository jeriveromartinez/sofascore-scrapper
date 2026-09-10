package events

import (
	"log/slog"
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

func EventToProto(e Event) *pb.SofaScoreEvent {
	homeTeam := TeamToProto(e.HomeTeamModel)
	awayTeam := TeamToProto(e.AwayTeamModel)

	// The proto type (pb.SofaScoreEvent) still carries the int64
	// SofaScoreEventId field. The model has moved to a string
	// ExternalMatchId. Bridge the two so the wire contract survives
	// until Task 2 renames the proto field to ExternalMatchId (string).
	// TODO: Task 2 will rename the proto field to external_match_id (string); the bridge must be removed at that point.
	sofaID, err := strconv.ParseInt(e.ExternalMatchId, 10, 64)
	if err != nil && e.ExternalMatchId != "" {
		// Empty string is the legitimate "unset" case and round-trips
		// to 0, matching pre-rename behavior. Any non-empty unparseable
		// value indicates a malformed source ID and would silently
		// corrupt the proto wire as a numeric 0 indistinguishable from
		// a legitimate zero — log loudly so the upstream is debuggable.
		slog.Default().Warn("events: unparseable ExternalMatchId in proto bridge",
			slog.Uint64("event_id", uint64(e.ID)),
			slog.String("external_match_id", e.ExternalMatchId),
			slog.Any("err", err),
		)
	}

	return &pb.SofaScoreEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   formatTime(e.CreatedAt),
		UpdatedAt:                   formatTime(e.UpdatedAt),
		SofaScoreEventId:            sofaID,
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
