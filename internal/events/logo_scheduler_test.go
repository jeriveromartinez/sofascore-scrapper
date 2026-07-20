package events

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func TestEnqueueTeamLogoDoesNotDropWorkWhenSemaphoreIsFull(t *testing.T) {
	var group singleflight.Group
	semaphore := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan int64, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	for teamID := int64(1); teamID <= 3; teamID++ {
		id := teamID
		enqueueTeamLogo(&group, semaphore, id, func() {
			defer wg.Done()
			started <- id
			<-release
		})
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first logo did not start")
	}
	close(release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queued logos were dropped or blocked forever")
	}
	if len(started) != 2 {
		t.Fatalf("remaining starts = %d, want 2", len(started))
	}
}
