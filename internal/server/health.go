package server

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type HealthCheckFunc func(context.Context) error

type HealthChecker struct {
	checks map[string]HealthCheckFunc
	mu     sync.RWMutex
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make(map[string]HealthCheckFunc),
	}
}

func (h *HealthChecker) Register(name string, fn HealthCheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = fn
}

func (h *HealthChecker) LivenessHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	}
}

func (h *HealthChecker) ReadinessHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		h.mu.RLock()
		checks := make(map[string]HealthCheckFunc, len(h.checks))
		for k, v := range h.checks {
			checks[k] = v
		}
		h.mu.RUnlock()

		var unhealthy []string
		for name, fn := range checks {
			if err := fn(c.Request.Context()); err != nil {
				unhealthy = append(unhealthy, name)
			}
		}

		if len(unhealthy) > 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":       "not ready",
				"dependencies": unhealthy,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
