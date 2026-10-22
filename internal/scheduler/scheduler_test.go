package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitRun(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

// TestSchedulerRunAndShutdownConcurrently mirrors app.Start, where Run
// and Shutdown execute in different errgroup goroutines. It must be
// clean under -race whichever call wins.
func TestSchedulerRunAndShutdownConcurrently(t *testing.T) {
	for i := 0; i < 20; i++ {
		s := New(discardLogger())
		done := make(chan error, 1)
		shutdownDone := make(chan struct{})
		go func() { done <- s.Run(context.Background()) }()
		go func() {
			s.Shutdown()
			close(shutdownDone)
		}()

		waitRun(t, done)
		select {
		case <-shutdownDone:
		case <-time.After(5 * time.Second):
			t.Fatal("Shutdown did not return")
		}
	}
}

func TestSchedulerRunAfterShutdownReturnsImmediately(t *testing.T) {
	s := New(discardLogger())
	s.Shutdown()

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	waitRun(t, done)
}

func TestSchedulerShutdownStopsRun(t *testing.T) {
	s := New(discardLogger())
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	time.Sleep(20 * time.Millisecond)
	s.Shutdown()
	waitRun(t, done)
}
