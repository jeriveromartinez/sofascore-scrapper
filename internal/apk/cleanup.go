package apk

import (
	"context"
	"fmt"
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
			cj.chunkStore.RemoveChunks(id.String())
			continue
		}

		if session.ExpiresAt.After(time.Now()) {
			continue
		}

		cj.chunkStore.RemoveChunks(id.String())

		if session.UserID > 0 {
			_ = cj.store.Abort(ctx, id)
		}
	}

	return nil
}
