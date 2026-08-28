package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	epochKey       = "events:current:v1:epoch"
	cacheKeyPrefix = "events:current:v1"
	defaultTTL     = 60 * time.Second
)

type CurrentEventsCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

type redisCache struct {
	client *goredis.Client
}

func NewCurrentEventsCache(client *goredis.Client) CurrentEventsCache {
	return &redisCache{client: client}
}

func (c *redisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if c.client == nil {
		return nil, false, nil
	}
	val, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (c *redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c.client == nil {
		return nil
	}
	return c.client.Set(ctx, key, value, ttl).Err()
}

type EpochStore struct {
	client *goredis.Client
}

func NewEpochStore(client *goredis.Client) *EpochStore {
	return &EpochStore{client: client}
}

func (e *EpochStore) Increment(ctx context.Context) (int64, error) {
	if e.client == nil {
		return 0, nil
	}
	return e.client.Incr(ctx, epochKey).Result()
}

func (e *EpochStore) Get(ctx context.Context) (int64, error) {
	if e.client == nil {
		return 0, nil
	}
	val, err := e.client.Get(ctx, epochKey).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}

func BuildCacheKey(epoch int64, tournamentIDs []uint, limit int) string {
	sorted := make([]uint, len(tournamentIDs))
	copy(sorted, tournamentIDs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sb strings.Builder
	for _, id := range sorted {
		sb.WriteString(fmt.Sprintf("%d,", id))
	}
	h := sha256.Sum256([]byte(sb.String()))
	hash := hex.EncodeToString(h[:])[:16]
	return fmt.Sprintf("%s:%d:%s:%d", cacheKeyPrefix, epoch, hash, limit)
}
