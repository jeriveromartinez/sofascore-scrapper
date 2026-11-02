package devices

import (
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

// DeviceToProto maps a device to its API representation. iptv_url carries
// the panel URL the device is served, which only the registration resolves
// (from the APK version published for the package); the list endpoints
// leave it empty.
func DeviceToProto(d Device) *pb.Device {
	pb := &pb.Device{
		Id:        uint32(d.ID),
		CreatedAt: formatDateTime(d.CreatedAt),
		UpdatedAt: formatDateTime(d.UpdatedAt),
		Token:     d.Token,
		Platform:  d.Platform,
		Name:      d.Name,
		Version:   d.Version,
		LastSeen:  d.LastSeen,
		PackageId: d.PackageId,
		Timezone:  d.Timezone,
		IptvUrl:   d.PanelURL,
	}
	if d.DomainID != nil {
		pb.DomainId = uint32(*d.DomainID)
	}
	return pb
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
