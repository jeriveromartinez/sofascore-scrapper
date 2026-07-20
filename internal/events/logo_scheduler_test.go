package events

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogoSchedulerProcessesWorkBeyondWorkerCount(t *testing.T) {
	scheduler := newLogoScheduler(logoWorkerCount)
	t.Cleanup(func() { scheduler.Shutdown(context.Background()) })

	const total = logoWorkerCount + 5
	completed := make(chan int64, total)
	for teamID := int64(1); teamID <= total; teamID++ {
		id := teamID
		scheduler.enqueue(id, func(context.Context) { completed <- id })
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
		scheduler.Shutdown(context.Background())
	})

	const total = logoWorkerCount * 2
	started := make(chan struct{}, total)
	completed := make(chan struct{}, total)
	var active atomic.Int64
	var maximum atomic.Int64
	for teamID := int64(1); teamID <= total; teamID++ {
		scheduler.enqueue(teamID, func(context.Context) {
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
		scheduler.Shutdown(context.Background())
	})

	started := make(chan struct{})
	var executions atomic.Int64
	scheduler.enqueue(1, func(context.Context) {
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
		scheduler.enqueue(1, func(context.Context) { executions.Add(1) })
	}
	releaseOnce.Do(func() { close(release) })
	scheduler.Shutdown(context.Background())

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
		scheduler.Shutdown(context.Background())
	})

	started := make(chan struct{}, logoWorkerCount)
	for teamID := int64(1); teamID <= logoWorkerCount; teamID++ {
		scheduler.enqueue(teamID, func(context.Context) {
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
		scheduler.enqueue(teamID, func(context.Context) { <-release })
	}
	runtime.Gosched()
	after := runtime.NumGoroutine()
	if growth := after - before; growth > 2 {
		t.Fatalf("queueing jobs added %d goroutines, want no growth tied to queue depth", growth)
	}

	releaseOnce.Do(func() { close(release) })
}

func TestLogoSchedulerShutdownFinishesActiveAndDiscardsQueuedWork(t *testing.T) {
	scheduler := NewLogoScheduler()
	release := make(chan struct{})
	started := make(chan struct{}, logoWorkerCount)
	completed := make(chan struct{}, logoWorkerCount)
	for teamID := int64(1); teamID <= logoWorkerCount; teamID++ {
		if !scheduler.enqueue(teamID, func(context.Context) {
			started <- struct{}{}
			<-release
			completed <- struct{}{}
		}) {
			t.Fatalf("active team %d was rejected", teamID)
		}
	}
	for range logoWorkerCount {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}

	var queuedRan atomic.Bool
	if !scheduler.enqueue(logoWorkerCount+1, func(context.Context) { queuedRan.Store(true) }) {
		t.Fatal("queued work was rejected before shutdown")
	}

	scheduler.Stop()
	if scheduler.enqueue(logoWorkerCount+2, func(context.Context) {}) {
		t.Fatal("work was accepted after shutdown began")
	}
	close(release)
	scheduler.Shutdown(context.Background())

	for range logoWorkerCount {
		select {
		case <-completed:
		default:
			t.Fatal("active work did not finish")
		}
	}
	if queuedRan.Load() {
		t.Fatal("queued work ran after shutdown began")
	}
}

func TestLogoSchedulerShutdownCancelsActiveWorkAtDeadline(t *testing.T) {
	scheduler := NewLogoScheduler()
	started := make(chan struct{})
	canceled := make(chan struct{})
	if !scheduler.enqueue(1, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
	}) {
		t.Fatal("active work was rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("work did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	scheduler.Shutdown(ctx)

	select {
	case <-canceled:
	default:
		t.Fatal("shutdown returned before canceled work exited")
	}
}
