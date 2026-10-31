package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingLiveCloser struct {
	calls atomic.Int64
}

func (c *countingLiveCloser) CloseStaleLive(_ context.Context, _ time.Time) (int64, error) {
	c.calls.Add(1)
	return 0, nil
}

func TestStartEventReconcile_CronDriven(t *testing.T) {
	orig := eventReconcileSpec
	eventReconcileSpec = "@every 1s"
	t.Cleanup(func() { eventReconcileSpec = orig })

	closer := &countingLiveCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := slog.New(slog.NewTextHandler(testLogWriter{t}, nil))
	runner := NewRunner(nil, logger)
	var wg sync.WaitGroup
	startEventReconcile(ctx, closer, runner, &wg, logger)

	time.Sleep(2500 * time.Millisecond)

	cancel()
	wg.Wait()

	if got := closer.calls.Load(); got < 2 {
		t.Errorf("CloseStaleLive called %d times in 2.5s with @every 1s spec, want >= 2 (cron is not driving the job)", got)
	}
}

func TestStartEventReconcile_DisabledWithoutCloser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := discardLogger()

	var wg sync.WaitGroup
	startEventReconcile(ctx, nil, NewRunner(nil, logger), &wg, logger)
	cancel()
	wg.Wait()
}
