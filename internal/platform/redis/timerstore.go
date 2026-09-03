package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// TimerEntry is the projection of a scheduled push timer that the
// index needs. Defined here to avoid importing internal/push from
// this low-level package.
type TimerEntry struct {
	DeviceID uint
	FireAt   time.Time
}

// TimerStore is the per-schedule scheduled-push timer index. The
// scheduler worker calls PopDue to find timers whose fire_at has
// arrived; the handler calls Enqueue when a new schedule is
// created; the worker calls Enqueue again when a recurring
// schedule re-arms itself.
//
// Durability contract:
//   - The authoritative state is the DB (scheduled_push_timers).
//   - Redis is a fast index; if it is wiped or unavailable the
//   worker falls back to ListDueTimers on the DB.
//   - On startup the app calls RebuildFromDB to repopulate the
//   index from the durable source.
type TimerStore interface {
	Enqueue(ctx context.Context, scheduleID uint, entries []TimerEntry) error
	PopDue(ctx context.Context, scheduleID uint, now time.Time, limit int) ([]TimerEntry, error)
	Remove(ctx context.Context, scheduleID uint) error
	Rebuild(ctx context.Context, snapshot map[uint][]TimerEntry) error
}

// RedisTimerStore is the production TimerStore. It uses a Redis
// ZSET per schedule: key = `schedule:{id}:timers`, member =
// device_id, score = unix seconds of fire_at.
type RedisTimerStore struct {
	client *goredis.Client
}

func NewTimerStore(client *goredis.Client) *RedisTimerStore {
	return &RedisTimerStore{client: client}
}

func scheduleKey(scheduleID uint) string {
	return fmt.Sprintf("schedule:%d:timers", scheduleID)
}

// Enqueue overwrites the schedule's ZSET with the given list.
// We use ZADD with the full set rather than appending so the
// caller can re-enqueue after a Redis wipe without leaking
// duplicates.
func (s *RedisTimerStore) Enqueue(ctx context.Context, scheduleID uint, entries []TimerEntry) error {
	if s == nil || s.client == nil {
		return nil
	}
	key := scheduleKey(scheduleID)
	pipe := s.client.Pipeline()
		pipe.Del(ctx, key)
	if len(entries) > 0 {
		members := make([]goredis.Z, 0, len(entries))
		for _, e := range entries {
			members = append(members, goredis.Z{
				Score:  float64(e.FireAt.Unix()),
				Member: strconv.FormatUint(uint64(e.DeviceID), 10),
			})
		}
		pipe.ZAdd(ctx, key, members...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// PopDue returns up to `limit` timer entries whose fire_at <= now
// for the given schedule. Entries are NOT removed from the ZSET:
// the worker removes them after a successful dispatch by calling
// RemoveScheduleTimer. We don't auto-remove so a worker crash
// leaves the timer in the index and the next worker tick picks it
// up.
func (s *RedisTimerStore) PopDue(ctx context.Context, scheduleID uint, now time.Time, limit int) ([]TimerEntry, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	key := scheduleKey(scheduleID)
	zs, err := s.client.ZRangeByScoreWithScores(ctx, key, &goredis.ZRangeBy{
		Min:    "-inf",
		Max:    strconv.FormatInt(now.Unix(), 10),
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("zrangebyscore %s: %w", key, err)
	}
	out := make([]TimerEntry, 0, len(zs))
	for _, z := range zs {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}
		id, perr := strconv.ParseUint(member, 10, 64)
		if perr != nil {
			continue
		}
		out = append(out, TimerEntry{
			DeviceID: uint(id),
			FireAt:   time.Unix(int64(z.Score), 0),
		})
	}
	return out, nil
}

// RemoveScheduleTimer deletes a single (schedule, device) entry
// from the ZSET. Called by the worker once it has successfully
// dispatched a timer (or marked it as fired in the DB).
func (s *RedisTimerStore) RemoveScheduleTimer(ctx context.Context, scheduleID uint, deviceID uint) error {
	if s == nil || s.client == nil {
		return nil
	}
	key := scheduleKey(scheduleID)
	return s.client.ZRem(ctx, key, strconv.FormatUint(uint64(deviceID), 10)).Err()
}

// Remove deletes the entire ZSET for a schedule. Called when the
// operator deletes the schedule or pauses it permanently.
func (s *RedisTimerStore) Remove(ctx context.Context, scheduleID uint) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Del(ctx, scheduleKey(scheduleID)).Err()
}

// Rebuild repopulates the index from a DB snapshot. Used at
// startup or after a Redis wipe to rebuild the cache from the
// authoritative source. The snapshot map is keyed by schedule_id.
func (s *RedisTimerStore) Rebuild(ctx context.Context, snapshot map[uint][]TimerEntry) error {
	if s == nil || s.client == nil {
		return nil
	}
	pipe := s.client.Pipeline()
	for scheduleID, entries := range snapshot {
		key := scheduleKey(scheduleID)
		pipe.Del(ctx, key)
		if len(entries) > 0 {
			members := make([]goredis.Z, 0, len(entries))
			for _, e := range entries {
				members = append(members, goredis.Z{
					Score:  float64(e.FireAt.Unix()),
					Member: strconv.FormatUint(uint64(e.DeviceID), 10),
				})
			}
			pipe.ZAdd(ctx, key, members...)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Client returns the underlying Redis client so tests and
// diagnostics can poke the connection.
func (s *RedisTimerStore) Client() *goredis.Client {
	if s == nil {
		return nil
	}
	return s.client
}