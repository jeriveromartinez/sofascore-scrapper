package events

import (
	"context"
	"encoding/json"
	"log"
)

type Service struct {
	repo  *Repository
	cache CurrentEventsCache
	epoch *EpochStore
}

func NewService(repo *Repository, cache CurrentEventsCache, epoch *EpochStore) *Service {
	return &Service{repo: repo, cache: cache, epoch: epoch}
}

func (s *Service) GetCurrentAndUpcoming(ctx context.Context, devID uint, limit int) ([]Event, error) {
	if limit <= 0 || limit > 6 {
		limit = 6
	}

	tournamentIDs, err := s.repo.ResolveTournamentIDs(ctx, devID)
	if err != nil {
		return nil, err
	}

	if s.cache != nil && len(tournamentIDs) > 0 {
		epoch, _ := s.epoch.Get(ctx)
		key := BuildCacheKey(epoch, tournamentIDs, limit)

		if data, hit, _ := s.cache.Get(ctx, key); hit {
			events, deserErr := deserializeEvents(data)
			if deserErr == nil {
				return events, nil
			}
			log.Printf("events: corrupt cache key %s, repopulating: %v", key, deserErr)
		}

		events, dbErr := s.repo.GetCurrentAndUpcoming(ctx, devID, limit)
		if dbErr != nil {
			return nil, dbErr
		}

		if data, serErr := serializeEvents(events); serErr == nil {
			_ = s.cache.Set(ctx, key, data, defaultTTL)
		}
		return events, nil
	}

	return s.repo.GetCurrentAndUpcoming(ctx, devID, limit)
}

func serializeEvents(events []Event) ([]byte, error) {
	return json.Marshal(events)
}

func deserializeEvents(data []byte) ([]Event, error) {
	var events []Event
	err := json.Unmarshal(data, &events)
	return events, err
}
