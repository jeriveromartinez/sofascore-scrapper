package reporting

import (
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

func EventStatsToProto(stats []EventStats) []*pb.EventStats {
	result := make([]*pb.EventStats, 0, len(stats))
	for _, s := range stats {
		result = append(result, &pb.EventStats{
			ExternalMatchId: s.ExternalMatchId,
			ViewCount:       s.ViewCount,
		})
	}
	return result
}
