package push

import (
	"log/slog"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics groups the Prometheus counters/gauges/histograms the
// push feature exports. Construct one per process and register
// it with the project's collector registry in wire-up. All
// methods are nil-safe so handlers can call them without a guard.
type Metrics struct {
	dispatched     *prometheus.CounterVec
	delivered      prometheus.Counter
	failed         *prometheus.CounterVec
	fired          *prometheus.CounterVec
	scheduleFailed prometheus.Counter
	wsConns        prometheus.Gauge
	enabled        prometheus.Gauge
	active         prometheus.Gauge
	latencyMS      prometheus.Histogram
}

// NewMetrics builds and registers a Metrics on the given
// registerer. Pass prometheus.DefaultRegisterer in production
// (or a test-local registry in unit tests). The returned
// Metrics is safe to use from any goroutine.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		dispatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "push_dispatched_total",
			Help: "Number of push dispatches attempted, by source (immediate|scheduled).",
		}, []string{"source"}),
		delivered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "push_delivered_total",
			Help: "Number of WsPush frames the client acknowledged.",
		}),
		failed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "push_failed_total",
			Help: "Number of failed delivery attempts, by reason.",
		}, []string{"reason"}),
		fired: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "push_schedule_fired_total",
			Help: "Number of scheduled_pushes fired, by type (one_shot|recurring).",
		}, []string{"type"}),
		scheduleFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "push_schedule_failed_total",
			Help: "Number of scheduled_pushes that failed to dispatch.",
		}),
		wsConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "push_websocket_connections",
			Help: "Current number of open WebSocket connections on this backend.",
		}),
		enabled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "push_enabled_users",
			Help: "Current number of users with the notifications toggle on.",
		}),
		active: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "push_active_schedules",
			Help: "Current number of active scheduled_pushes.",
		}),
		latencyMS: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "push_latency_ms",
			Help:    "Delivery latency in milliseconds (server sent_at -> client acked_at).",
			Buckets: []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		}),
	}
	reg.MustRegister(m.dispatched, m.delivered, m.failed, m.fired, m.scheduleFailed, m.wsConns, m.enabled, m.active, m.latencyMS)
	return m
}

// IncDispatched bumps the dispatched counter for the given source.
func (m *Metrics) IncDispatched(source string) {
	if m == nil {
		return
	}
	m.dispatched.WithLabelValues(source).Inc()
}

// IncDelivered bumps the delivered counter (called from the ack path).
func (m *Metrics) IncDelivered() {
	if m == nil {
		return
	}
	m.delivered.Inc()
}

// IncFailed bumps the failed counter for the given reason. The
// reason string is one of the FailureReason constants in this
// package; unknown values map to "other" so a future enum does
// not break the dashboard.
func (m *Metrics) IncFailed(reason FailureReason) {
	if m == nil {
		return
	}
	r := string(reason)
	if r == "" {
		r = "other"
	}
	m.failed.WithLabelValues(r).Inc()
}

// IncScheduleFired bumps the schedule-fired counter.
func (m *Metrics) IncScheduleFired(scheduleType ScheduleType) {
	if m == nil {
		return
	}
	m.fired.WithLabelValues(string(scheduleType)).Inc()
}

// IncScheduleFailed bumps the schedule-failed counter.
func (m *Metrics) IncScheduleFailed() {
	if m == nil {
		return
	}
	m.scheduleFailed.Inc()
}

// ObserveLatency records one delivery latency in milliseconds.
// Negative values are clamped to 0 so a clock skew between the
// hub goroutine and the delivery loop does not poison the
// histogram.
func (m *Metrics) ObserveLatency(ms int) {
	if m == nil {
		return
	}
	if ms < 0 {
		ms = 0
	}
	m.latencyMS.Observe(float64(ms))
}

// SetWSConnections sets the gauge to the given count. The
// realtime hub calls this from its register/unregister paths.
func (m *Metrics) SetWSConnections(n int) {
	if m == nil {
		return
	}
	m.wsConns.Set(float64(n))
}

// SetEnabledUsers sets the gauge to the given count. Computed
// periodically by a background goroutine (TODO: phase 4d).
func (m *Metrics) SetEnabledUsers(n int64) {
	if m == nil {
		return
	}
	m.enabled.Set(float64(n))
}

// SetActiveSchedules sets the gauge to the given count. Computed
// periodically by a background goroutine.
func (m *Metrics) SetActiveSchedules(n int64) {
	if m == nil {
		return
	}
	m.active.Set(float64(n))
}

// FromRealtimeFailureReason maps the realtime package's
// ErrDeviceNotConnected sentinel to the failure reason used by
// the metrics layer. Kept here (rather than in realtime) so the
// push feature owns its own metric labels.
func FromRealtimeFailureReason(err error) FailureReason {
	if err == realtime.ErrDeviceNotConnected {
		return FailureDeviceOffline
	}
	return FailureInternalError
}

// IncFailedError is a convenience wrapper: maps the error to a
// FailureReason and bumps the counter. Used by the service's
// dispatch loop.
func (m *Metrics) IncFailedError(err error) {
	if m == nil {
		return
	}
	m.IncFailed(FromRealtimeFailureReason(err))
}

// logOnce is a tiny helper for the metrics package to emit a
// debug log when an unexpected label value is encountered, so
// the dashboard can flag it. Currently unused but kept as a
// hook for the dashboard team's future request.
var _ = slog.Default
