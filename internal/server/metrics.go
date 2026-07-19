package server

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		observability.HttpInFlight.Inc()
		defer observability.HttpInFlight.Dec()

		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		observability.HttpRequestsTotal.WithLabelValues(method, route, status).Inc()
		observability.HttpRequestDuration.WithLabelValues(method, route, status).Observe(time.Since(start).Seconds())
	}
}

func MetricsHandler() gin.HandlerFunc {
	h := promhttp.HandlerFor(observability.Gatherer(), promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

type SQLCollector struct {
	db *sql.DB
}

func NewSQLCollector(db *sql.DB) *SQLCollector {
	return &SQLCollector{db: db}
}

func (c *SQLCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *SQLCollector) Collect(ch chan<- prometheus.Metric) {
	if c.db == nil {
		return
	}
	stats := c.db.Stats()

	ch <- prometheus.MustNewConstMetric(
		observability.SQLOpenConnections.Desc(),
		prometheus.GaugeValue,
		float64(stats.OpenConnections),
	)

	ch <- prometheus.MustNewConstMetric(
		observability.SQLInUseConnections.Desc(),
		prometheus.GaugeValue,
		float64(stats.InUse),
	)

	ch <- prometheus.MustNewConstMetric(
		observability.SQLWaitCount.Desc(),
		prometheus.CounterValue,
		float64(stats.WaitCount),
	)
}

type RedisCollector struct {
	client redis.UniversalClient
}

func NewRedisCollector(client redis.UniversalClient) *RedisCollector {
	return &RedisCollector{client: client}
}

func (c *RedisCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *RedisCollector) Collect(ch chan<- prometheus.Metric) {
	if c.client == nil {
		return
	}
	stats := c.client.PoolStats()

	ch <- prometheus.MustNewConstMetric(
		observability.RedisPoolHits.Desc(),
		prometheus.CounterValue,
		float64(stats.Hits),
	)

	ch <- prometheus.MustNewConstMetric(
		observability.RedisPoolMisses.Desc(),
		prometheus.CounterValue,
		float64(stats.Misses),
	)

	ch <- prometheus.MustNewConstMetric(
		observability.RedisPoolTimeouts.Desc(),
		prometheus.CounterValue,
		float64(stats.Timeouts),
	)
}
