package observability

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsRouteNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		HttpInFlight.Inc()
		defer HttpInFlight.Dec()

		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		HttpRequestsTotal.WithLabelValues(method, route, status).Inc()
		HttpRequestDuration.WithLabelValues(method, route, status).Observe(time.Since(start).Seconds())
	})
	router.GET("/users/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	ts := httptest.NewServer(router)
	defer ts.Close()

	for i := 0; i < 100; i++ {
		resp, err := http.Get(ts.URL + "/users/" + strconv.Itoa(i))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}

	family, err := Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var found bool
	var rawRoutes []string
	for _, mf := range family {
		if mf.GetName() == "http_requests_total" {
			found = true
			for _, m := range mf.GetMetric() {
				labels := map[string]string{}
				for _, lp := range m.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				route := labels["route"]
				if route == "" {
					continue
				}
				if strings.Contains(route, "/users/") && !strings.Contains(route, "/users/:id") {
					rawRoutes = append(rawRoutes, route)
				}
			}
		}
	}
	if !found {
		t.Error("http_requests_total metric not found")
	}
	if len(rawRoutes) > 0 {
		t.Errorf("found %d raw URL labels instead of /users/:id: %v", len(rawRoutes), rawRoutes)
	}
}
