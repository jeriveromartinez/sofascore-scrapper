package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var reg = prometheus.NewRegistry()

func init() {
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

func Register(c prometheus.Collector) error {
	return reg.Register(c)
}

func Gatherer() prometheus.Gatherer {
	return reg
}

var (
	HttpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "route", "status"},
	)

	HttpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	HttpInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_in_flight_requests",
			Help: "Current in-flight HTTP requests.",
		},
	)

	SQLOpenConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "sql_open_connections",
			Help: "Number of open SQL connections.",
		},
	)

	SQLInUseConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "sql_in_use_connections",
			Help: "Number of SQL connections currently in use.",
		},
	)

	SQLWaitCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sql_wait_count_total",
			Help: "Total number of connections waited for.",
		},
	)

	RedisPoolHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_pool_hits_total",
			Help: "Total number of Redis pool hits.",
		},
	)

	RedisPoolMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_pool_misses_total",
			Help: "Total number of Redis pool misses.",
		},
	)

	RedisPoolTimeouts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "redis_pool_timeouts_total",
			Help: "Total number of Redis pool timeouts.",
		},
	)

	ScrapeRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scrape_requests_total",
			Help: "Total number of scrape requests.",
		},
		[]string{"sport"},
	)

	ScrapeEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scrape_events_total",
			Help: "Total number of events scraped.",
		},
		[]string{"sport"},
	)

	ScrapeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scrape_duration_seconds",
			Help:    "Duration of scrape operations.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"sport"},
	)

	SchedulerJobRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scheduler_job_runs_total",
			Help: "Total number of scheduler job runs.",
		},
		[]string{"job"},
	)

	SchedulerJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scheduler_job_duration_seconds",
			Help:    "Duration of scheduler jobs.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"job"},
	)

	SchedulerLockContention = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scheduler_lock_contention_total",
			Help: "Total number of scheduler lock contention events.",
		},
		[]string{"job"},
	)

	UploadActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "upload_active",
			Help: "Number of active uploads.",
		},
	)

	UploadBytes = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "upload_bytes_total",
			Help: "Total bytes uploaded.",
		},
	)

	UploadFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "upload_failures_total",
			Help: "Total number of upload failures.",
		},
	)

	EventCacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "event_cache_hits_total",
			Help: "Total number of event cache hits.",
		},
	)

	EventCacheMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "event_cache_misses_total",
			Help: "Total number of event cache misses.",
		},
	)

	APKDownloadsBuffered = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "apk_downloads_buffered",
			Help: "Number of buffered APK download counts.",
		},
	)
)

func init() {
	reg.MustRegister(HttpRequestsTotal)
	reg.MustRegister(HttpRequestDuration)
	reg.MustRegister(HttpInFlight)
	reg.MustRegister(ScrapeRequests)
	reg.MustRegister(ScrapeEvents)
	reg.MustRegister(ScrapeDuration)
	reg.MustRegister(SchedulerJobRuns)
	reg.MustRegister(SchedulerJobDuration)
	reg.MustRegister(SchedulerLockContention)
	reg.MustRegister(UploadActive)
	reg.MustRegister(UploadBytes)
	reg.MustRegister(UploadFailures)
	reg.MustRegister(EventCacheHits)
	reg.MustRegister(EventCacheMisses)
	reg.MustRegister(APKDownloadsBuffered)
}
