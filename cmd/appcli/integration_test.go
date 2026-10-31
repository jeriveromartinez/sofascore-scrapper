package main

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/app"
	"github.com/jeriveromartinez/scores365-scrapper/internal/auth"
	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/realtime"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestSimulatorAgainstTheRealAPI drives the simulator against the
// production router: the same protobuf wire format, the same device
// authentication and the same WebSocket handshake the Flutter client
// uses.
func TestSimulatorAgainstTheRealAPI(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&users.User{},
		&domains.Domain{},
		&apk.ApkVersion{},
		&devices.Device{},
		&tournaments.Tournament{},
		&tournaments.GlobalTournamentConfig{},
		&tournaments.DeviceTournament{},
		&events.Team{},
		&events.Event{},
	))

	owner := users.User{Email: "owner@test.local", Password: "x", Role: users.RoleAdmin}
	require.NoError(t, db.Create(&owner).Error)

	domain := domains.Domain{Domain: "panel.example.com", UserID: owner.ID}
	require.NoError(t, db.Create(&domain).Error)

	apkRow := apk.ApkVersion{
		Version:       "1.0.30",
		FileName:      "app.apk",
		FilePath:      "/tmp/app.apk",
		PackageName:   DefaultPackage,
		IsActive:      true,
		DownloadToken: "0f9a1c2e-3b4d-4e5f-8a9b-0c1d2e3f4a5b",
		IPTVUrl:       "https://panel.example.com",
	}
	require.NoError(t, db.Create(&apkRow).Error)

	league := tournaments.Tournament{Name: "Liga MX", Slug: "liga-mx"}
	require.NoError(t, db.Create(&league).Error)
	require.NoError(t, db.Create(&tournaments.GlobalTournamentConfig{TournamentID: league.ID}).Error)

	home := events.Team{TeamId: 100, Name: "Local"}
	away := events.Team{TeamId: 200, Name: "Visitante"}
	require.NoError(t, db.Create(&home).Error)
	require.NoError(t, db.Create(&away).Error)

	require.NoError(t, db.Create(&events.Event{
		ExternalMatchId:             "fotmob-4711",
		Source:                      "fotmob",
		Sport:                       "football",
		HomeScore:                   2,
		AwayScore:                   1,
		HomeTeamId:                  100,
		AwayTeamId:                  200,
		LeagueId:                    league.ID,
		StatusType:                  "inprogress",
		StartTimestamp:              time.Now().Unix(),
		CurrentPeriodStartTimestamp: time.Now().Unix(),
	}).Error)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	require.NoError(t, err)

	router := app.NewRouter(db, redisClient, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}, tokens, nil, realtime.NewHub(), nil, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)
	require.Equal(t, 0, code, "stderr: %s", stderr.String())

	var device devices.Device
	require.NoError(t, db.Where("token = ?", "sim-token").First(&device).Error)
	assert.Equal(t, domain.ID, *device.DomainID)
	assert.Equal(t, DefaultPackage, device.PackageId)

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"events", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)
	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Local")
	assert.Contains(t, stdout.String(), "Visitante")
	assert.Contains(t, stdout.String(), "2 - 1")
	assert.Contains(t, stdout.String(), "Liga MX")

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"ws", "--base-url", srv.URL, "--token", "sim-token", "--duration", "1s"}, &stdout, &stderr)
	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "conectado")
	assert.Contains(t, stdout.String(), fmt.Sprintf("device #%d", device.ID))
}
