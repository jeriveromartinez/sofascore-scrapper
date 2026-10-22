package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
	"github.com/prometheus/client_golang/prometheus"
)

// metricsJobSeq disambiguates the job label used by the metric tests.
// The collectors are process-wide, so a fixed label would accumulate
// across repeated runs of the same test and make the assertions
// depend on how many times the package had already run.
var metricsJobSeq atomic.Uint64

func metricsJob(prefix string) string {
	return fmt.Sprintf("%s:%d", prefix, metricsJobSeq.Add(1))
}

// jobMetric gathers collector through a private registry and returns
// the counter value or histogram sample count of the series for job.
// prometheus/testutil is not used because it pulls modules absent from
// go.mod.
func jobMetric(t *testing.T, collector prometheus.Collector, job string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() != "job" || label.GetValue() != job {
					continue
				}
				if h := metric.GetHistogram(); h != nil {
					return float64(h.GetSampleCount())
				}
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestRunLockedRecordsJobRunMetrics(t *testing.T) {
	job := metricsJob("test:metrics:run")
	runner := NewRunner(&fakeLocker{lease: &fakeLease{}, acquired: true}, slog.Default())

	if err := runner.RunLocked(context.Background(), job, time.Minute, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("RunLocked: %v", err)
	}

	if got := jobMetric(t, observability.SchedulerJobRuns, job); got != 1 {
		t.Fatalf("job runs = %v, want 1", got)
	}
	if got := jobMetric(t, observability.SchedulerJobDuration, job); got != 1 {
		t.Fatalf("job duration samples = %v, want 1", got)
	}
	if got := jobMetric(t, observability.SchedulerLockContention, job); got != 0 {
		t.Fatalf("lock contention = %v, want 0", got)
	}
}

func TestRunLockedWithoutLockerRecordsJobRunMetrics(t *testing.T) {
	job := metricsJob("test:metrics:nolocker")
	runner := NewRunner(nil, slog.Default())

	if err := runner.RunLocked(context.Background(), job, time.Minute, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("RunLocked: %v", err)
	}

	if got := jobMetric(t, observability.SchedulerJobRuns, job); got != 1 {
		t.Fatalf("job runs = %v, want 1", got)
	}
	if got := jobMetric(t, observability.SchedulerJobDuration, job); got != 1 {
		t.Fatalf("job duration samples = %v, want 1", got)
	}
}

func TestRunLockedRecordsLockContention(t *testing.T) {
	job := metricsJob("test:metrics:contention")
	runner := NewRunner(&fakeLocker{lease: &fakeLease{}, acquired: false}, slog.Default())

	ran := false
	if err := runner.RunLocked(context.Background(), job, time.Minute, func(context.Context) error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("RunLocked: %v", err)
	}

	if ran {
		t.Fatal("job ran without holding the lock")
	}
	if got := jobMetric(t, observability.SchedulerLockContention, job); got != 1 {
		t.Fatalf("lock contention = %v, want 1", got)
	}
	if got := jobMetric(t, observability.SchedulerJobRuns, job); got != 0 {
		t.Fatalf("job runs = %v, want 0", got)
	}
}
