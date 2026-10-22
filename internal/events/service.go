package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
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
		epoch, err := s.epoch.Get(ctx)
		if err != nil || epoch == 0 {
			// Redis unreachable / no epoch yet: bypass the cache
			// entirely. With epoch=0 every device would key off
			// the same entry, so a stale value from before the
			// outage would silently serve every request until the
			// epoch key reappears.
			return s.repo.getCurrentAndUpcoming(ctx, tournamentIDs, limit)
		}
		key := BuildCacheKey(epoch, tournamentIDs, limit)

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result := s.currentSF.DoChan(key, func() (interface{}, error) {
			// The shared load must not inherit the caller's
			// cancellation. Every concurrent caller waits on this
			// one flight, so a single client disconnecting would
			// otherwise abort the database query and make the
			// other callers fail with "context canceled" even
			// though they are still connected. The worker keeps
			// the caller's context values and is bounded by the
			// flight timeout; the disconnected caller itself
			// returns ctx.Err() through the select below.
			flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), currentEventsFlightTimeout)
			defer cancel()

			// The single-flight worker re-checks the cache under
			// flightCtx so a request that lost the outer race
			// (e.g. concurrent first call) still hits Redis rather
			// than hammering the DB. The earlier outer cache.Get
			// was redundant: it always returned miss whenever the
			// worker had to fall through to the DB anyway.
			if data, hit, _ := s.cache.Get(flightCtx, key); hit {
				events, deserErr := deserializeEvents(data)
				if deserErr == nil {
					observability.EventCacheHits.Inc()
					return events, nil
				}
				slog.Default().WarnContext(flightCtx, "events: corrupt cache entry, repopulating",
					slog.String("key", key),
					slog.String("err", deserErr.Error()),
				)
			}
			observability.EventCacheMisses.Inc()

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
