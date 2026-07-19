package app

import (
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

func TestRouteCompatibility(t *testing.T) {
	cfg := config.Config{JWTSecret: "test-secret"}
	router := NewRouter(nil, cfg)
	got := make(map[string]bool)
	for _, route := range router.Routes() {
		got[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/app/v1/update",
		"GET /api/app/v1/apk/download/:token",
		"GET /api/app/v1/current-events",
		"POST /api/app/v1/devices",
		"POST /api/app/v1/devices/viewing",
		"GET /api/app/v1/devices/url/:packageName",
		"POST /api/app/v1/crash-report",
		"GET /api/app/v1/teams/logo/:teamId",
		"POST /api/web/v1/users/register",
		"POST /api/web/v1/users/login",
		"POST /api/web/v1/users/refresh",
		"POST /api/web/v1/users/logout",
		"GET /api/web/v1/users",
		"GET /api/web/v1/users/:id",
		"POST /api/web/v1/users",
		"PUT /api/web/v1/users/:id",
		"DELETE /api/web/v1/users/:id",
		"GET /api/web/v1/events",
		"GET /api/web/v1/devices",
		"GET /api/web/v1/devices/all",
		"PUT /api/web/v1/devices",
		"GET /api/web/v1/playback",
		"GET /api/web/v1/stats/top-events",
		"POST /api/web/v1/apk/upload",
		"POST /api/web/v1/apk/upload/chunk",
		"POST /api/web/v1/apk/upload/assemble",
		"GET /api/web/v1/apk/versions",
		"PUT /api/web/v1/apk/:id",
		"GET /api/web/v1/tournaments",
		"GET /api/web/v1/tournaments/:id",
		"POST /api/web/v1/tournaments",
		"PUT /api/web/v1/tournaments/:id",
		"DELETE /api/web/v1/tournaments/:id",
		"GET /api/web/v1/device-tournaments",
		"GET /api/web/v1/device-tournaments/:deviceId",
		"POST /api/web/v1/device-tournaments",
		"DELETE /api/web/v1/device-tournaments",
		"PUT /api/web/v1/device-tournaments/:deviceId",
		"GET /api/web/v1/global-tournament-config",
		"POST /api/web/v1/global-tournament-config",
		"DELETE /api/web/v1/global-tournament-config/:tournamentId",
		"GET /api/web/v1/domains",
		"GET /api/web/v1/domains/:id",
		"POST /api/web/v1/domains",
		"PUT /api/web/v1/domains/:id",
		"DELETE /api/web/v1/domains/:id",
	} {
		if !got[want] {
			t.Errorf("missing route %s", want)
		}
	}
	if len(got) != 46 {
		t.Fatalf("got %d routes, want 46", len(got))
	}
}
