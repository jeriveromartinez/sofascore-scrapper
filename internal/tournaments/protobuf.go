package tournaments

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func TournamentToProto(t Tournament) *pb.Tournament {
	return &pb.Tournament{
		Id:        uint32(t.ID),
		CreatedAt: formatDateTime(t.CreatedAt),
		UpdatedAt: formatDateTime(t.UpdatedAt),
		Name:      t.Name,
		Slug:      t.Slug,
		Region:    t.Region,
	}
}

func TournamentPtrToProto(t *Tournament) *pb.Tournament {
	if t == nil {
		return nil
	}
	return TournamentToProto(*t)
}

func TournamentsToProto(ts []Tournament) []*pb.Tournament {
	result := make([]*pb.Tournament, 0, len(ts))
	for _, t := range ts {
		result = append(result, TournamentToProto(t))
	}
	return result
}

func GlobalConfigToProto(g GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	return &pb.GlobalTournamentConfig{
		Id:           uint32(g.ID),
		CreatedAt:    formatDateTime(g.CreatedAt),
		UpdatedAt:    formatDateTime(g.UpdatedAt),
		TournamentId: uint32(g.TournamentID),
		Tournament:   TournamentPtrToProto(g.Tournament),
	}
}

func GlobalConfigPtrToProto(g *GlobalTournamentConfig) *pb.GlobalTournamentConfig {
	if g == nil {
		return nil
	}
	return GlobalConfigToProto(*g)
}

func GlobalConfigsToProto(gs []GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, GlobalConfigToProto(g))
	}
	return result
}

func GlobalConfigPtrsToProto(gs []*GlobalTournamentConfig) []*pb.GlobalTournamentConfig {
	result := make([]*pb.GlobalTournamentConfig, 0, len(gs))
	for _, g := range gs {
		result = append(result, GlobalConfigPtrToProto(g))
	}
	return result
}

func DeviceTournamentToProto(dt DeviceTournament) *pb.DeviceTournament {
	p := &pb.DeviceTournament{
		Id:           uint32(dt.ID),
		CreatedAt:    formatDateTime(dt.CreatedAt),
		UpdatedAt:    formatDateTime(dt.UpdatedAt),
		DeviceId:     uint32(dt.DeviceID),
		TournamentId: uint32(dt.TournamentID),
	}
	if dt.Tournament != nil {
		p.Tournament = TournamentToProto(*dt.Tournament)
	}
	if dt.Device != nil {
		p.Device = devices.DeviceToProto(*dt.Device)
	}
	return p
}

func DeviceTournamentsToProto(dts []DeviceTournament) []*pb.DeviceTournament {
	result := make([]*pb.DeviceTournament, 0, len(dts))
	for _, dt := range dts {
		result = append(result, DeviceTournamentToProto(dt))
	}
	return result
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
