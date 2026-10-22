package push

import (
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
	"github.com/prometheus/client_golang/prometheus"
)

func gatheredFamilies(t *testing.T, g prometheus.Gatherer) map[string]bool {
	t.Helper()
	families, err := g.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, f := range families {
		names[f.GetName()] = true
	}
	return names
}

// TestNewMetrics_RegistersWithGivenRegisterer proves the push
// collectors end up in the registry the caller passes, and not in
// prometheus.DefaultRegisterer, which the /metrics endpoint does not
// serve.
func TestNewMetrics_RegistersWithGivenRegisterer(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	m.IncDispatched("immediate")

	families := gatheredFamilies(t, reg)
	if !families["push_dispatched_total"] {
		t.Fatal("push_dispatched_total missing from the given registry")
	}
	if !families["push_websocket_connections"] {
		t.Fatal("push_websocket_connections missing from the given registry")
	}

	defaultFamilies := gatheredFamilies(t, prometheus.DefaultGatherer)
	if defaultFamilies["push_dispatched_total"] {
		t.Fatal("push metrics leaked into prometheus.DefaultRegisterer")
	}
}

// TestNewMetrics_VisibleThroughServedRegistry proves the metrics are
// exposed by the registry the /metrics endpoint actually serves.
func TestNewMetrics_VisibleThroughServedRegistry(t *testing.T) {
	m := NewMetrics(observability.Registerer())
	m.IncScheduleFired(ScheduleTypeRecurring)

	families := gatheredFamilies(t, observability.Gatherer())
	if !families["push_schedule_fired_total"] {
		t.Fatal("push_schedule_fired_total missing from the served registry")
	}
	if !families["push_dispatched_total"] {
		t.Fatal("push_dispatched_total missing from the served registry")
	}
}

// TestNewMetrics_NilRegistererIsSafe covers the nil-guard: the
// constructor must not panic when the caller has no registry.
func TestNewMetrics_NilRegistererIsSafe(t *testing.T) {
	m := NewMetrics(nil)
	m.IncDispatched("immediate")
	m.IncDelivered()
	m.IncFailed(FailureInternalError)
	m.IncScheduleFired(ScheduleTypeOneShot)
	m.IncScheduleFailed()
	m.SetWSConnections(3)
	m.SetEnabledUsers(4)
	m.SetActiveSchedules(5)
	m.ObserveLatency(12)
}
