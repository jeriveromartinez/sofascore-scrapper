package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
	"github.com/prometheus/client_golang/prometheus"
)

// pushGaugeValue reads a gauge from the registry the push metrics
// were registered with.
func pushGaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		metrics := f.GetMetric()
		if len(metrics) == 0 {
			t.Fatalf("%s has no metrics", name)
		}
		return metrics[0].GetGauge().GetValue()
	}
	t.Fatalf("%s missing from the registry", name)
	return 0
}

// TestStartPushGaugesRefreshesTheCounters proves the background
// refresher feeds the two process-wide push gauges with real counts
// instead of leaving them at 0.
func TestStartPushGaugesRefreshesTheCounters(t *testing.T) {
	f := newTimersUnitFixture(t, &unitPusher{})
	f.seedDueSchedule(t)

	reg := prometheus.NewRegistry()
	f.svc.SetMetrics(push.NewMetrics(reg))

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	startPushGauges(ctx, f.svc, 10*time.Millisecond, slog.Default(), &wg)
	defer func() {
		cancel()
		wg.Wait()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pushGaugeValue(t, reg, "push_enabled_users") == 1 &&
			pushGaugeValue(t, reg, "push_active_schedules") == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("gauges not refreshed: enabled_users=%v active_schedules=%v",
		pushGaugeValue(t, reg, "push_enabled_users"),
		pushGaugeValue(t, reg, "push_active_schedules"))
}
