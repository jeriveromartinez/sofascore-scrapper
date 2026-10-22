package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestRunScheduledJobReportsLeaseFailure asserts a lease failure
// reaches the log. The cron callbacks used to discard the error
// RunLocked returns, so a Redis outage that stopped a job from
// running was invisible.
func TestRunScheduledJobReportsLeaseFailure(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	runner := NewRunner(&fakeLocker{acquireEr: errors.New("redis unavailable")}, logger)

	runScheduledJob(context.Background(), runner, logger, "daily stats", lockStatsDaily, time.Minute, func(context.Context) error {
		t.Fatal("the job must not run when the lease cannot be acquired")
		return nil
	})

	out := logs.String()
	if !strings.Contains(out, lockStatsDaily) {
		t.Fatalf("the lease that failed is missing from the log: %s", out)
	}
	if !strings.Contains(out, "redis unavailable") {
		t.Fatalf("the lease error is missing from the log: %s", out)
	}
}

// TestRunScheduledJobReportsJobFailure asserts the error a job
// returns is reported with the schedule that produced it.
func TestRunScheduledJobReportsJobFailure(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	runner := NewRunner(&fakeLocker{lease: &fakeLease{}, acquired: true}, logger)

	runScheduledJob(context.Background(), runner, logger, "monthly stats", lockStatsMonthly, time.Minute, func(context.Context) error {
		return errors.New("aggregation failed")
	})

	out := logs.String()
	if !strings.Contains(out, lockStatsMonthly) {
		t.Fatalf("the lease that failed is missing from the log: %s", out)
	}
	if !strings.Contains(out, "aggregation failed") {
		t.Fatalf("the job error is missing from the log: %s", out)
	}
}

// TestRunScheduledJobStaysSilentOnSuccess asserts a successful run
// does not produce an error record.
func TestRunScheduledJobStaysSilentOnSuccess(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	runner := NewRunner(&fakeLocker{lease: &fakeLease{}, acquired: true}, logger)

	runScheduledJob(context.Background(), runner, logger, "uploads cleanup", lockUploadsCleanup, time.Minute, func(context.Context) error {
		return nil
	})

	if out := logs.String(); out != "" {
		t.Fatalf("a successful run was logged: %s", out)
	}
}
