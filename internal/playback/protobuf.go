package playback

import (
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func PlaybackToProto(p *PlaybackLog) *pb.PlaybackLog {
	if p == nil {
		return nil
	}
	return &pb.PlaybackLog{
		Id:        uint32(p.ID),
		CreatedAt: formatTime(p.CreatedAt),
		UpdatedAt: formatTime(p.UpdatedAt),
		DeviceId:  uint32(p.DeviceID),
		Content:   p.Content,
		StartedAt: p.StartedAt,
		EndedAt:   p.EndedAt,
	}
}

func PlaybackListToProto(pl []*PlaybackLog, total int64) *pb.PlaybackLogList {
	result := make([]*pb.PlaybackLog, 0, len(pl))
	for _, p := range pl {
		result = append(result, PlaybackToProto(p))
	}
	return &pb.PlaybackLogList{List: result, Total: uint32(total)}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
