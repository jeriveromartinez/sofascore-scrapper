package api

import (
	"fmt"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func deviceToProto(d models.Device) *pb.Device {
	return &pb.Device{
		Id:        uint32(d.ID),
		CreatedAt: formatTime(d.CreatedAt),
		UpdatedAt: formatTime(d.UpdatedAt),
		Token:     d.Token,
		Platform:  d.Platform,
		Name:      d.Name,
		LastSeen:  d.LastSeen,
	}
}

func devicesToProto(devices []models.Device) []*pb.Device {
	result := make([]*pb.Device, 0, len(devices))
	for _, d := range devices {
		result = append(result, deviceToProto(d))
	}
	return result
}

func tournamentToProto(t models.Tournament) *pb.Tournament {
	return &pb.Tournament{
		Id:        uint32(t.ID),
		CreatedAt: formatTime(t.CreatedAt),
		UpdatedAt: formatTime(t.UpdatedAt),
		Name:      t.Name,
		Slug:      t.Slug,
		Region:    t.Region,
	}
}

func tournamentPtrToProto(t *models.Tournament) *pb.Tournament {
	if t == nil {
		return nil
	}
	return tournamentToProto(*t)
}

func tournamentsToProto(ts []models.Tournament) []*pb.Tournament {
	result := make([]*pb.Tournament, 0, len(ts))
	for _, t := range ts {
		result = append(result, tournamentToProto(t))
	}
	return result
}

func teamPtrToProto(t *models.Team) *pb.Team {
	if t == nil {
		return nil
	}
	return &pb.Team{
		Id:      uint32(t.ID),
		TeamId:  t.TeamId,
		LogoUrl: t.LogoUrl,
	}
}

func eventToProto(e models.SofaScoreEvent) *pb.SofaScoreEvent {
	return &pb.SofaScoreEvent{
		Id:                          uint32(e.ID),
		CreatedAt:                   formatTime(e.CreatedAt),
		UpdatedAt:                   formatTime(e.UpdatedAt),
		SofaScoreEventId:            e.SofaScoreEventId,
		Sport:                       e.Sport,
		HomeTeam:                    e.HomeTeam,
		HomeScore:                   int32(e.HomeScore),
		HomeTeamId:                  e.HomeTeamId,
		AwayTeam:                    e.AwayTeam,
		AwayScore:                   int32(e.AwayScore),
		AwayTeamId:                  e.AwayTeamId,
		ScrapedAt:                   e.ScrapedAt,
		StartTimestamp:              e.StartTimestamp,
		CurrentPeriodStartTimestamp: e.CurrentPeriodStartTimestamp,
		Slug:                        e.Slug,
		TeamHome:                    teamPtrToProto(e.HomeTeamModel),
		TeamAway:                    teamPtrToProto(e.AwayTeamModel),
		League:                      tournamentPtrToProto(e.League),
	}
}

func eventsToProto(events []models.SofaScoreEvent) []*pb.SofaScoreEvent {
	result := make([]*pb.SofaScoreEvent, 0, len(events))
	for _, e := range events {
		result = append(result, eventToProto(e))
	}
	return result
}

func playbackToProto(p *models.PlaybackLog) *pb.PlaybackLog {
	if p == nil {
		return nil
	}
	return &pb.PlaybackLog{
		Id:               uint32(p.ID),
		CreatedAt:        formatTime(p.CreatedAt),
		UpdatedAt:        formatTime(p.UpdatedAt),
		DeviceId:         uint32(p.DeviceID),
		SofaScoreEventId: p.SofaScoreEventId,
		StartedAt:        p.StartedAt,
		EndedAt:          p.EndedAt,
	}
}

func globalConfigToProto(g models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	return &pb.GlobalTournamentConfig{
		Id:           uint32(g.ID),
		CreatedAt:    formatTime(g.CreatedAt),
		UpdatedAt:    formatTime(g.UpdatedAt),
		TournamentId: uint32(g.TournamentID),
		Tournament:   tournamentPtrToProto(g.Tournament),
	}
}

func globalConfigPtrToProto(g *models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	if g == nil {
		return nil
	}
	return globalConfigToProto(*g)
}

func globalConfigsToProto(gs []models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, globalConfigToProto(g))
	}
	return result
}

func globalConfigPtrsToProto(gs []*models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, globalConfigPtrToProto(g))
	}
	return result
}

func deviceTournamentToProto(dt models.DeviceTournament) *pb.DeviceTournament {
	p := &pb.DeviceTournament{
		Id:           uint32(dt.ID),
		CreatedAt:    formatTime(dt.CreatedAt),
		UpdatedAt:    formatTime(dt.UpdatedAt),
		DeviceId:     uint32(dt.DeviceID),
		TournamentId: uint32(dt.TournamentID),
	}
	if dt.Tournament != nil {
		p.Tournament = tournamentToProto(*dt.Tournament)
	}
	if dt.Device != nil {
		p.Device = deviceToProto(*dt.Device)
	}
	return p
}

func deviceTournamentsToProto(dts []models.DeviceTournament) []*pb.DeviceTournament {
	result := make([]*pb.DeviceTournament, 0, len(dts))
	for _, dt := range dts {
		result = append(result, deviceTournamentToProto(dt))
	}
	return result
}

func eventStatsToProto(stats []repository.EventStats) []*pb.EventStats {
	result := make([]*pb.EventStats, 0, len(stats))
	for _, s := range stats {
		result = append(result, &pb.EventStats{
			SofaScoreEventId: s.SofaScoreEventId,
			ViewCount:        s.ViewCount,
		})
	}
	return result
}

func apkToProto(v models.ApkVersion, downloadURL string) *pb.ApkInfo {
	return &pb.ApkInfo{
		Id:               uint32(v.ID),
		Version:          v.Version,
		FileName:         v.FileName,
		FileSize:         v.FileSize,
		Description:      v.Description,
		IsActive:         v.IsActive,
		PackageName:      v.PackageName,
		VersionCode:      v.VersionCode,
		MinSdkVersion:    v.MinSDKVersion,
		TargetSdkVersion: v.TargetSDKVersion,
		DownloadToken:    v.DownloadToken,
		DownloadUrl:      downloadURL,
		CreatedAt:        formatTime(v.CreatedAt),
	}
}

func apksToProto(versions []models.ApkVersion) []*pb.ApkInfo {
	result := make([]*pb.ApkInfo, 0, len(versions))
	for _, v := range versions {
		result = append(result, apkToProto(v, fmt.Sprintf("/api/v1/apk/download/%s", v.DownloadToken)))
	}
	return result
}
