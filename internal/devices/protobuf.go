package devices

import (
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func DeviceToProto(d Device) *pb.Device {
	return &pb.Device{
		Id:        uint32(d.ID),
		CreatedAt: formatDateTime(d.CreatedAt),
		UpdatedAt: formatDateTime(d.UpdatedAt),
		Token:     d.Token,
		Platform:  d.Platform,
		Name:      d.Name,
		Version:   d.Version,
		LastSeen:  d.LastSeen,
	}
}

func DevicesToProto(devices []Device) []*pb.Device {
	result := make([]*pb.Device, 0, len(devices))
	for _, d := range devices {
		result = append(result, DeviceToProto(d))
	}
	return result
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
