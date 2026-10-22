package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	redisplatform "github.com/jeriveromartinez/scores365-scrapper/internal/platform/redis"
)

type fakeLease struct {
	renewErr error
	renews   atomic.Int32
}

func (l *fakeLease) Key() string   { return "fake-lock" }
func (l *fakeLease) Owner() string { return "fake-owner" }

func (l *fakeLease) Renew(ctx context.Context, ttl time.Duration) error {
	l.renews.Add(1)
	return l.renewErr
}

func (l *fakeLease) Release(ctx context.Context) error { return nil }

type fakeLocker struct {
	lease     *fakeLease
	acquired  bool
	acquireEr error
}

func (l *fakeLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (redisplatform.Lease, bool, error) {
	if l.acquireEr != nil {
		return nil, false, l.acquireEr
	}
	return l.lease, l.acquired, nil
}

// TestRunLockedReturnsWhenJobCompletes guards the deadlock in which the
// lease-renewal goroutine only exited on jobCtx.Done() while jobCancel
// was deferred until after wg.Wait(): RunLocked never returned, the
// lock was renewed forever and every scheduled job ran exactly once.
func TestRunLockedReturnsWhenJobCompletes(t *testing.T) {
	lease := &fakeLease{}
	runner := NewRunner(&fakeLocker{lease: lease, acquired: true}, slog.Default())

	done := make(chan error, 1)
	go func() {
		done <- runner.RunLocked(context.Background(), "test:returns", 30*time.Millisecond, func(context.Context) error {
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLocked: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLocked did not return after the job completed: the lease renewal goroutine never exits")
	}
}

// TestRunLockedKeepsJobContextAlive asserts the job context is not
// cancelled while the job runs: cancelling the renewal goroutine must
// not race the job itself.
func TestRunLockedKeepsJobContextAlive(t *testing.T) {
	lease := &fakeLease{}
	runner := NewRunner(&fakeLocker{lease: lease, acquired: true}, slog.Default())

	done := make(chan error, 1)
	go func() {
		done <- runner.RunLocked(context.Background(), "test:ctx", 30*time.Millisecond, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("job context was cancelled while the job was running: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLocked did not return after the job completed")
	}
}

// TestRunLockedReportsRenewFailure asserts a failed lease renewal is
// surfaced to the caller instead of being swallowed.
func TestRunLockedReportsRenewFailure(t *testing.T) {
	lease := &fakeLease{renewErr: errors.New("renew boom")}
	runner := NewRunner(&fakeLocker{lease: lease, acquired: true}, slog.Default())

	done := make(chan error, 1)
	go func() {
		done <- runner.RunLocked(context.Background(), "test:renew", 30*time.Millisecond, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the renewal failure to be reported")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLocked did not return after the lease renewal failed")
	}
}
