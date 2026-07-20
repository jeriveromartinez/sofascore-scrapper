package events

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogoSchedulerProcessesWorkBeyondWorkerCount(t *testing.T) {
	scheduler := newLogoScheduler(logoWorkerCount)
	t.Cleanup(scheduler.shutdown)

	const total = logoWorkerCount + 5
	completed := make(chan int64, total)
	for teamID := int64(1); teamID <= total; teamID++ {
		id := teamID
		scheduler.enqueue(id, func() { completed <- id })
	}

	seen := make(map[int64]bool, total)
	for range total {
		select {
		case teamID := <-completed:
			seen[teamID] = true
		case <-time.After(time.Second):
			t.Fatalf("completed %d unique jobs, want %d", len(seen), total)
		}
	}
	if len(seen) != total {
		t.Fatalf("completed %d unique jobs, want %d", len(seen), total)
	}
}

func TestLogoSchedulerLimitsActiveWork(t *testing.T) {
	scheduler := newLogoScheduler(logoWorkerCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		scheduler.shutdown()
	})

	const total = logoWorkerCount * 2
	started := make(chan struct{}, total)
	completed := make(chan struct{}, total)
	var active atomic.Int64
	var maximum atomic.Int64
	for teamID := int64(1); teamID <= total; teamID++ {
		scheduler.enqueue(teamID, func() {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			completed <- struct{}{}
		})
	}

	for range logoWorkerCount {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d jobs became active", logoWorkerCount)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	for range total {
		select {
		case <-completed:
		case <-time.After(time.Second):
			t.Fatal("queued jobs did not complete")
		}
	}
	if got := maximum.Load(); got > logoWorkerCount {
		t.Fatalf("maximum active jobs = %d, want at most %d", got, logoWorkerCount)
	}
}

func TestLogoSchedulerCoalescesDuplicateTeamIDs(t *testing.T) {
	scheduler := newLogoScheduler(logoWorkerCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		scheduler.shutdown()
	})

	started := make(chan struct{})
	var executions atomic.Int64
	scheduler.enqueue(1, func() {
		executions.Add(1)
		close(started)
		<-release
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("logo job did not start")
	}

	for range 100 {
		scheduler.enqueue(1, func() { executions.Add(1) })
	}
	releaseOnce.Do(func() { close(release) })
	scheduler.shutdown()

	if got := executions.Load(); got != 1 {
		t.Fatalf("duplicate team executed %d times, want 1", got)
	}
}

func TestLogoSchedulerQueuedWorkDoesNotCreateGoroutines(t *testing.T) {
	scheduler := newLogoScheduler(logoWorkerCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		scheduler.shutdown()
	})

	started := make(chan struct{}, logoWorkerCount)
	for teamID := int64(1); teamID <= logoWorkerCount; teamID++ {
		scheduler.enqueue(teamID, func() {
			started <- struct{}{}
			<-release
		})
	}
	for range logoWorkerCount {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}

	before := runtime.NumGoroutine()
	for teamID := int64(logoWorkerCount + 1); teamID <= 1000; teamID++ {
		scheduler.enqueue(teamID, func() { <-release })
	}
	runtime.Gosched()
	after := runtime.NumGoroutine()
	if growth := after - before; growth > 2 {
		t.Fatalf("queueing jobs added %d goroutines, want no growth tied to queue depth", growth)
	}

	releaseOnce.Do(func() { close(release) })
}
