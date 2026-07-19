package common

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
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

func TeamPtrToProto(t *events.Team) *pb.Team {
	return events.TeamToProto(t)
}

func EventToProto(e events.Event) *pb.SofaScoreEvent {
	return events.EventToProto(e)
}

func EventsToProto(evs []events.Event) []*pb.SofaScoreEvent {
	result := make([]*pb.SofaScoreEvent, 0, len(evs))
	for _, e := range evs {
		if e.AwayTeamModel != nil {
			e.AwayTeamModel.LogoUrl = "/api/app/v1" + e.AwayTeamModel.LogoUrl
		}
		if e.HomeTeamModel != nil {
			e.HomeTeamModel.LogoUrl = "/api/app/v1" + e.HomeTeamModel.LogoUrl
		}
		result = append(result, events.EventToProto(e))
	}
	return result
}

func PlaybackToProto(p *models.PlaybackLog) *pb.PlaybackLog {
	return playback.PlaybackToProto(p)
}

func PlaybackListToProto(pl []*models.PlaybackLog, total int64) *pb.PlaybackLogList {
	return playback.PlaybackListToProto(pl, total)
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

func EventStatsToProto(stats []reporting.EventStats) []*pb.EventStats {
	return reporting.EventStatsToProto(stats)
}

func ApkToProto(v models.ApkVersion, downloadURL string) *pb.ApkInfo {
	return apk.ApkToProto(v, downloadURL)
}

func ApksToProto(versions []models.ApkVersion) []*pb.ApkInfo {
	return apk.ApksToProto(versions)
}
