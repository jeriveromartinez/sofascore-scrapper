package apk

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type CleanupJob struct {
	store      *UploadStateStore
	chunkStore *ChunkStore
}

func NewCleanupJob(store *UploadStateStore, chunkStore *ChunkStore) *CleanupJob {
	return &CleanupJob{
		store:      store,
		chunkStore: chunkStore,
	}
}

const (
	cleanupBatchSize = 50
)

func (cj *CleanupJob) Run(ctx context.Context, client *goredis.Client) error {
	removed, err := cj.chunkStore.CleanupStaleFiles(time.Now(), ChunkTempMaxAge, ChunkDirMaxAge)
	if err != nil {
		return fmt.Errorf("cleanup stale chunk files: %w", err)
	}
	if removed > 0 {
		slog.Default().Info("apk: removed stale upload leftovers",
			slog.Int("entries", removed))
	}

	if client == nil {
		return nil
	}

	expired, err := cj.store.ListExpired(ctx, time.Now(), cleanupBatchSize)
	if err != nil {
		return fmt.Errorf("list expired: %w", err)
	}

	for _, id := range expired {
		session, err := cj.store.Get(ctx, id)
		if err != nil {
			if err := cj.chunkStore.RemoveChunks(id.String()); err != nil {
				slog.Default().Warn("cleanup: remove the chunks of an unreadable upload session",
					slog.String("upload_id", id.String()),
					slog.String("err", err.Error()),
				)
			}
			continue
		}

		if session.ExpiresAt.After(time.Now()) {
			continue
		}

		if err := cj.chunkStore.RemoveChunks(id.String()); err != nil {
			slog.Default().Warn("cleanup: remove the chunks of an expired upload session",
				slog.String("upload_id", id.String()),
				slog.String("err", err.Error()),
			)
		}

		if session.UserID > 0 {
			_ = cj.store.Abort(ctx, id)
		}
	}

	return nil
}
