package common

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func UserToProto(u models.User) *pb.User {
	return users.UserToProto(u)
}

func UsersToProto(us []models.User) []*pb.User {
	return users.UsersToProto(us)
}

func DomainToProto(d models.Domain) *pb.Domain {
	return domains.DomainToProto(d)
}

func DomainsToProto(ds []models.Domain) []*pb.Domain {
	return domains.DomainsToProto(ds)
}

func DeviceToProto(d models.Device) *pb.Device {
	return devices.DeviceToProto(d)
}

func DevicesToProto(ds []models.Device) []*pb.Device {
	return devices.DevicesToProto(ds)
}

func TournamentToProto(t models.Tournament) *pb.Tournament {
	return tournaments.TournamentToProto(t)
}

func TournamentPtrToProto(t *models.Tournament) *pb.Tournament {
	return tournaments.TournamentPtrToProto(t)
}

func TournamentsToProto(ts []models.Tournament) []*pb.Tournament {
	return tournaments.TournamentsToProto(ts)
}

func TeamPtrToProto(t *models.Team) *pb.Team {
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

func EventToProto(e models.SofaScoreEvent) *pb.SofaScoreEvent {
	return &pb.SofaScoreEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   FormatTime(e.CreatedAt),
		UpdatedAt:                   FormatTime(e.UpdatedAt),
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
		TeamHome:                    TeamPtrToProto(e.HomeTeamModel),
		TeamAway:                    TeamPtrToProto(e.AwayTeamModel),
		League:                      TournamentPtrToProto(e.League),
	}
}

func EventsToProto(events []models.SofaScoreEvent) []*pb.SofaScoreEvent {
	result := make([]*pb.SofaScoreEvent, 0, len(events))
	for _, e := range events {
		e.AwayTeamModel.LogoUrl = "/api/app/v1" + e.AwayTeamModel.LogoUrl
		e.HomeTeamModel.LogoUrl = "/api/app/v1" + e.HomeTeamModel.LogoUrl
		result = append(result, EventToProto(e))
	}
	return result
}

func PlaybackToProto(p *models.PlaybackLog) *pb.PlaybackLog {
	if p == nil {
		return nil
	}
	return &pb.PlaybackLog{
		Id:        uint32(p.ID),
		CreatedAt: FormatTime(p.CreatedAt),
		UpdatedAt: FormatTime(p.UpdatedAt),
		DeviceId:  uint32(p.DeviceID),
		Content:   p.Content,
		StartedAt: p.StartedAt,
		EndedAt:   p.EndedAt,
	}
}

func PlaybackListToProto(pl []*models.PlaybackLog, total int64) *pb.PlaybackLogList {
	result := make([]*pb.PlaybackLog, 0, len(pl))
	for _, p := range pl {
		result = append(result, PlaybackToProto(p))
	}
	return &pb.PlaybackLogList{List: result, Total: uint32(total)}
}

func GlobalConfigToProto(g models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	return tournaments.GlobalConfigToProto(g)
}

func GlobalConfigPtrToProto(g *models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	return tournaments.GlobalConfigPtrToProto(g)
}

func GlobalConfigsToProto(gs []models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	return tournaments.GlobalConfigsToProto(gs)
}

func GlobalConfigPtrsToProto(gs []*models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	return tournaments.GlobalConfigPtrsToProto(gs)
}

func DeviceTournamentToProto(dt models.DeviceTournament) *pb.DeviceTournament {
	return tournaments.DeviceTournamentToProto(dt)
}

func DeviceTournamentsToProto(dts []models.DeviceTournament) []*pb.DeviceTournament {
	return tournaments.DeviceTournamentsToProto(dts)
}

func EventStatsToProto(stats []repository.EventStats) []*pb.EventStats {
	result := make([]*pb.EventStats, 0, len(stats))
	for _, s := range stats {
		result = append(result, &pb.EventStats{
			SofaScoreEventId: s.SofaScoreEventId,
			ViewCount:        s.ViewCount,
		})
	}
	return result
}

func ApkToProto(v models.ApkVersion, downloadURL string) *pb.ApkInfo {
	return apk.ApkToProto(v, downloadURL)
}

func ApksToProto(versions []models.ApkVersion) []*pb.ApkInfo {
	return apk.ApksToProto(versions)
}
