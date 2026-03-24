package common

import (
	"fmt"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func DeviceToProto(d models.Device) *pb.Device {
	return &pb.Device{
		Id:        uint32(d.ID),
		CreatedAt: FormatTime(d.CreatedAt),
		UpdatedAt: FormatTime(d.UpdatedAt),
		Token:     d.Token,
		Platform:  d.Platform,
		Name:      d.Name,
		LastSeen:  d.LastSeen,
	}
}

func DevicesToProto(devices []models.Device) []*pb.Device {
	result := make([]*pb.Device, 0, len(devices))
	for _, d := range devices {
		result = append(result, DeviceToProto(d))
	}
	return result
}

func TournamentToProto(t models.Tournament) *pb.Tournament {
	return &pb.Tournament{
		Id:        uint32(t.ID),
		CreatedAt: FormatTime(t.CreatedAt),
		UpdatedAt: FormatTime(t.UpdatedAt),
		Name:      t.Name,
		Slug:      t.Slug,
		Region:    t.Region,
	}
}

func TournamentPtrToProto(t *models.Tournament) *pb.Tournament {
	if t == nil {
		return nil
	}
	return TournamentToProto(*t)
}

func TournamentsToProto(ts []models.Tournament) []*pb.Tournament {
	result := make([]*pb.Tournament, 0, len(ts))
	for _, t := range ts {
		result = append(result, TournamentToProto(t))
	}
	return result
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

func GlobalConfigToProto(g models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	return &pb.GlobalTournamentConfig{
		Id:           uint32(g.ID),
		CreatedAt:    FormatTime(g.CreatedAt),
		UpdatedAt:    FormatTime(g.UpdatedAt),
		TournamentId: uint32(g.TournamentID),
		Tournament:   TournamentPtrToProto(g.Tournament),
	}
}

func GlobalConfigPtrToProto(g *models.GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	if g == nil {
		return nil
	}
	return GlobalConfigToProto(*g)
}

func GlobalConfigsToProto(gs []models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, GlobalConfigToProto(g))
	}
	return result
}

func GlobalConfigPtrsToProto(gs []*models.GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, GlobalConfigPtrToProto(g))
	}
	return result
}

func DeviceTournamentToProto(dt models.DeviceTournament) *pb.DeviceTournament {
	p := &pb.DeviceTournament{
		Id:           uint32(dt.ID),
		CreatedAt:    FormatTime(dt.CreatedAt),
		UpdatedAt:    FormatTime(dt.UpdatedAt),
		DeviceId:     uint32(dt.DeviceID),
		TournamentId: uint32(dt.TournamentID),
	}
	if dt.Tournament != nil {
		p.Tournament = TournamentToProto(*dt.Tournament)
	}
	if dt.Device != nil {
		p.Device = DeviceToProto(*dt.Device)
	}
	return p
}

func DeviceTournamentsToProto(dts []models.DeviceTournament) []*pb.DeviceTournament {
	result := make([]*pb.DeviceTournament, 0, len(dts))
	for _, dt := range dts {
		result = append(result, DeviceTournamentToProto(dt))
	}
	return result
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
		CreatedAt:        FormatTime(v.CreatedAt),
	}
}

func ApksToProto(versions []models.ApkVersion) []*pb.ApkInfo {
	result := make([]*pb.ApkInfo, 0, len(versions))
	for _, v := range versions {
		result = append(result, ApkToProto(v, fmt.Sprintf("/api/app/v1/apk/download/%s", v.DownloadToken)))
	}
	return result
}
