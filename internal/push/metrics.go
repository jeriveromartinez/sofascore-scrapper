package push

import (
	"errors"

	"github.com/jeriveromartinez/scores365-scrapper/internal/realtime"
	"github.com/prometheus/client_golang/prometheus"
)

// The collectors are package-level singletons on purpose: the
// registerer the caller hands to NewMetrics is usually process-wide,
// so registering the same collector twice (two App instances in one
// test binary, for instance) must not blow up. Reusing one instance
// also keeps the values visible in whichever registry ends up serving
// /metrics.
var (
	dispatchedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "push_dispatched_total",
		Help: "Number of push dispatches attempted, by source (immediate|scheduled).",
	}, []string{"source"})
	deliveredCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "push_delivered_total",
		Help: "Number of WsPush frames the client acknowledged.",
	})
	failedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "push_failed_total",
		Help: "Number of failed delivery attempts, by reason.",
	}, []string{"reason"})
	firedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "push_schedule_fired_total",
		Help: "Number of scheduled_pushes fired, by type (one_shot|recurring).",
	}, []string{"type"})
	scheduleFailedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "push_schedule_failed_total",
		Help: "Number of scheduled_pushes that failed to dispatch.",
	})
	wsConnsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "push_websocket_connections",
		Help: "Current number of open WebSocket connections on this backend.",
	})
	enabledGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "push_enabled_users",
		Help: "Current number of users with the notifications toggle on.",
	})
	activeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "push_active_schedules",
		Help: "Current number of active scheduled_pushes.",
	})
	latencyHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "push_latency_ms",
		Help:    "Delivery latency in milliseconds (server sent_at -> client acked_at).",
		Buckets: []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	})
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

// NewMetrics builds a Metrics and registers its collectors with the
// given registerer. The registerer MUST be the one the /metrics
// endpoint serves (the project's own collector registry), otherwise
// the push metrics are collected but never scraped.
//
// The returned Metrics is safe to use from any goroutine.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		dispatched:     dispatchedCounter,
		delivered:      deliveredCounter,
		failed:         failedCounter,
		fired:          firedCounter,
		scheduleFailed: scheduleFailedCounter,
		wsConns:        wsConnsGauge,
		enabled:        enabledGauge,
		active:         activeGauge,
		latencyMS:      latencyHistogram,
	}
	if reg == nil {
		return m
	}
	for _, c := range []prometheus.Collector{
		m.dispatched, m.delivered, m.failed, m.fired, m.scheduleFailed,
		m.wsConns, m.enabled, m.active, m.latencyMS,
	} {
		if err := reg.Register(c); err != nil {
			var already prometheus.AlreadyRegisteredError
			if !errors.As(err, &already) {
				panic(err)
			}
		}
	}
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
	m.AddFailed(reason, 1)
}

// AddFailed bumps the failed counter for the given reason by n, with
// the same label mapping as IncFailed.
func (m *Metrics) AddFailed(reason FailureReason, n int64) {
	if m == nil {
		return
	}
	r := string(reason)
	if r == "" {
		r = "other"
	}
	m.failed.WithLabelValues(r).Add(float64(n))
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

// SetEnabledUsers sets the gauge to the given count. Refreshed
// periodically from the database by Service.RefreshGauges.
func (m *Metrics) SetEnabledUsers(n int64) {

	if m == nil {
		return
	}
	m.enabled.Set(float64(n))
}

// SetActiveSchedules sets the gauge to the given count. Refreshed
// periodically from the database by Service.RefreshGauges.
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
