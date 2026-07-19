package events

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"golang.org/x/sync/singleflight"
)

type Service struct {
	repo      *Repository
	cache     CurrentEventsCache
	epoch     *EpochStore
	currentSF singleflight.Group
}

const currentEventsFlightTimeout = 30 * time.Second

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

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result := s.currentSF.DoChan(key, func() (interface{}, error) {
			flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), currentEventsFlightTimeout)
			defer cancel()

			if data, hit, _ := s.cache.Get(flightCtx, key); hit {
				events, deserErr := deserializeEvents(data)
				if deserErr == nil {
					return events, nil
				}
				log.Printf("events: corrupt cache key %s, repopulating: %v", key, deserErr)
			}

			events, dbErr := s.repo.getCurrentAndUpcoming(flightCtx, tournamentIDs, limit)
			if dbErr != nil {
				return nil, dbErr
			}

			if data, serErr := serializeEvents(events); serErr == nil {
				_ = s.cache.Set(flightCtx, key, data, defaultTTL)
			}
			return events, nil
		})

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case loaded := <-result:
			if loaded.Err != nil {
				return nil, loaded.Err
			}
			return loaded.Val.([]Event), nil
		}
	}

	return s.repo.getCurrentAndUpcoming(ctx, tournamentIDs, limit)
}

func serializeEvents(events []Event) ([]byte, error) {
	return json.Marshal(events)
}

func deserializeEvents(data []byte) ([]Event, error) {
	var events []Event
	err := json.Unmarshal(data, &events)
	return events, err
}
