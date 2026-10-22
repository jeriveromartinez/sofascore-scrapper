package playback

import (
	"context"
	"errors"
	"testing"
)

func TestGetListHonoursContext(t *testing.T) {
	repo := NewRepository(setupTestDB(t))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.GetList(cancelled, 1, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetList error = %v, want context.Canceled", err)
	}
}
