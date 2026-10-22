package apk

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// metricValue reads a counter or gauge directly. prometheus/testutil is
// not used because it pulls a module that go.mod does not carry.
func metricValue(t *testing.T, m prometheus.Metric) float64 {
	t.Helper()
	var out dto.Metric
	if err := m.Write(&out); err != nil {
		t.Fatalf("read metric: %v", err)
	}
	switch {
	case out.Counter != nil:
		return out.Counter.GetValue()
	case out.Gauge != nil:
		return out.Gauge.GetValue()
	}
	t.Fatalf("metric is neither a counter nor a gauge")
	return 0
}
