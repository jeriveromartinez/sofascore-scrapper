package reporting

import (
	"strconv"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func EventStatsToProto(stats []EventStats) []*pb.EventStats {
	result := make([]*pb.EventStats, 0, len(stats))
	for _, s := range stats {
		result = append(result, &pb.EventStats{
			ExternalMatchId: strconv.FormatInt(s.SofaScoreEventId, 10),
			ViewCount:       s.ViewCount,
		})
	}
	return result
}
