package domains

import (
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
)

func DomainToProto(d Domain) *pb.Domain {
	p := &pb.Domain{
		Id:        uint32(d.ID),
		CreatedAt: formatDateTime(d.CreatedAt),
		UpdatedAt: formatDateTime(d.UpdatedAt),
		Domain:    d.Domain,
		UserId:    uint32(d.UserID),
	}
	if d.User != nil {
		p.User = users.UserToProto(*d.User)
	}
	return p
}

func DomainsToProto(domains []Domain) []*pb.Domain {
	result := make([]*pb.Domain, 0, len(domains))
	for _, domain := range domains {
		result = append(result, DomainToProto(domain))
	}
	return result
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
