package apk

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	activeDownloadsKey = "apk:downloads:active"
	pendingPrefix      = "apk:downloads:pending:"
	flushLeaseKey      = "scheduler:lock:apk-downloads:flush"
	flushLeaseTTL      = 10 * time.Minute
)

var flushRenameScript = goredis.NewScript(`
	if redis.call("EXISTS", KEYS[1]) == 1 then
		return redis.call("RENAME", KEYS[1], ARGV[1])
	end
	return 0
`)

type DownloadCounter interface {
	Increment(ctx context.Context, apkID uint) error
	Flush(ctx context.Context) error
	ReprocessOrphans(ctx context.Context) error
}

type downloadCounter struct {
	client *goredis.Client
	db     *gorm.DB
	locker redisplatform.Locker
}

func NewDownloadCounter(client *goredis.Client, db *gorm.DB) DownloadCounter {
	return &downloadCounter{
		client: client,
		db:     db,
		locker: redisplatform.NewLocker(client),
	}
}

func (c *downloadCounter) Increment(ctx context.Context, apkID uint) error {
	return c.client.HIncrBy(ctx, activeDownloadsKey, strconv.FormatUint(uint64(apkID), 10), 1).Err()
}

func (c *downloadCounter) Flush(ctx context.Context) error {
	return c.flushWithProbe(ctx, nil)
}

func (c *downloadCounter) flushWithProbe(ctx context.Context, afterRename func()) error {
	if c.client == nil {
		return nil
	}

	lease, acquired, err := c.locker.Acquire(ctx, flushLeaseKey, flushLeaseTTL)
	if err != nil {
		return fmt.Errorf("acquire flush lease: %w", err)
	}
	if !acquired {
		return nil
	}

	if err := c.flushUnderLease(ctx, afterRename); err != nil {
		err = fmt.Errorf("flush failed: %w", err)
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = lease.Release(releaseCtx)
	return err
}

func (c *downloadCounter) flushUnderLease(ctx context.Context, afterRename func()) error {
	batchID := uuid.New().String()
	pendingKey := pendingPrefix + batchID

	renamed, err := flushRenameScript.Run(ctx, c.client, []string{activeDownloadsKey}, pendingKey).Bool()
	if err != nil {
		return fmt.Errorf("rename active to pending: %w", err)
	}
	if !renamed {
		return nil
	}

	if afterRename != nil {
		afterRename()
	}

	return c.processPending(ctx, batchID, pendingKey)
}

func (c *downloadCounter) processPending(ctx context.Context, batchID, pendingKey string) error {
	deltas, err := c.client.HGetAll(ctx, pendingKey).Result()
	if err != nil {
		return fmt.Errorf("hgetall pending: %w", err)
	}

	err = c.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Table("download_counter_flushes").Create(map[string]string{
			"batch_id": batchID,
		})
		if result.Error != nil {
			return fmt.Errorf("insert batch %s: %w", batchID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("batch %s already processed", batchID)
		}

		for field, countStr := range deltas {
			count, err := strconv.ParseInt(countStr, 10, 64)
			if err != nil {
				return fmt.Errorf("parse count for apk %s: %w", field, err)
			}
			if count <= 0 {
				continue
			}
			id, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return fmt.Errorf("parse apk id %s: %w", field, err)
			}
			if err := tx.Model(&ApkVersion{}).Where("id = ?", id).
				Update("total_downloads", gorm.Expr("total_downloads + ?", count)).Error; err != nil {
				return fmt.Errorf("increment apk %d by %d: %w", id, count, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	if err := c.client.Del(ctx, pendingKey).Err(); err != nil {
		return fmt.Errorf("delete pending key: %w", err)
	}
	return nil
}

func (c *downloadCounter) reprocessBatch(ctx context.Context, batchID, pendingKey string) error {
	return c.processPending(ctx, batchID, pendingKey)
}

func (c *downloadCounter) ReprocessOrphans(ctx context.Context) error {
	if c.client == nil {
		return nil
	}

	iter := c.client.Scan(ctx, 0, pendingPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		pendingKey := iter.Val()
		batchID := pendingKey[len(pendingPrefix):]
		if err := c.processPending(ctx, batchID, pendingKey); err != nil {
			return fmt.Errorf("reprocess batch %s: %w", batchID, err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan pending keys: %w", err)
	}
	return nil
}
