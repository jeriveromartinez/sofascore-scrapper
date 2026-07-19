package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, cfg config.Config, tokens *auth.TokenService) *gin.Engine {
	router := gin.New()
	router.Use(server.CORS(), gin.Logger(), gin.Recovery())

	appV1 := router.Group("/api/app/v1")
	webV1 := router.Group("/api/web/v1")

	appMw := devices.AppMiddleware(db)
	authMw := auth.AuthMiddleware(tokens)

	apkRepo := apk.NewRepository(db)
	apkAppHandler := apk.NewAppHandler(apkRepo)
	apkAppHandler.RegisterRoutes(appV1)

	apkAdminHandler := apk.NewAdminHandler(apkRepo)
	apkAdminHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: authMw})

	devRepo := devices.NewRepository(db)
	playbackRepo := playback.NewRepository(db)

	devicesAppHandler := devices.NewAppHandler(devRepo)
	devicesAppHandler.RegisterRoutes(appV1)

	playbackAppHandler := playback.NewAppHandler(playbackRepo, devRepo)
	playbackAppHandler.RegisterRoutes(appV1, playback.PlaybackAppHandlerDeps{AppMiddleware: appMw})

	devicesAdminHandler := devices.NewAdminHandler(devRepo)
	devicesAdminHandler.RegisterRoutes(webV1, devices.AdminHandlerDeps{AuthMiddleware: authMw})

	playbackAdminHandler := playback.NewAdminHandler(playbackRepo)
	playbackAdminHandler.RegisterRoutes(webV1, playback.AdminHandlerDeps{AuthMiddleware: authMw})

	reportingRepo := reporting.NewRepository(db)
	crashHandler := reporting.NewCrashHandler(reportingRepo)
	crashHandler.RegisterRoutes(appV1)

	statsHandler := reporting.NewStatsHandler(reportingRepo)
	statsHandler.RegisterRoutes(webV1, reporting.StatsHandlerDeps{AuthMiddleware: authMw})

	eventsRepo := events.NewRepository(db)
	eventsAppHandler := events.NewAppHandler(eventsRepo)
	eventsAppHandler.RegisterRoutes(appV1, events.AppHandlerDeps{AppMiddleware: appMw})

	eventsAdminHandler := events.NewAdminHandler(db)
	eventsAdminHandler.RegisterRoutes(webV1, events.AdminHandlerDeps{AuthMiddleware: authMw})

	logoHandler := events.NewLogoHandler()
	logoHandler.RegisterRoutes(appV1)

	userRepo := users.NewRepository(db)
	userHandler := users.NewHandler(userRepo)
	userHandler.RegisterUserRoutes(webV1, users.HandlerDeps{AuthMiddleware: authMw})

	authRepo := auth.NewAuthRepository(db)
	authHandler := auth.NewAuthHandler(authRepo, userRepo, tokens)
	authHandler.RegisterAuthRoutes(webV1)

	tournamentRepo := tournaments.NewRepository(db)
	tournamentHandler := tournaments.NewHandler(tournamentRepo)
	tournamentHandler.RegisterRoutes(webV1, tournaments.HandlerDeps{AuthMiddleware: authMw})

	deviceAssignmentsRepo := tournaments.NewDeviceAssignmentsRepository(db)
	deviceAssignmentsHandler := tournaments.NewDeviceAssignmentsHandler(deviceAssignmentsRepo)
	deviceAssignmentsHandler.RegisterRoutes(webV1, tournaments.DeviceAssignmentsHandlerDeps{AuthMiddleware: authMw})

	globalConfigRepo := tournaments.NewGlobalConfigRepository(db)
	globalConfigHandler := tournaments.NewGlobalConfigHandler(globalConfigRepo)
	globalConfigHandler.RegisterRoutes(webV1, tournaments.GlobalConfigHandlerDeps{AuthMiddleware: authMw})

	domainRepo := domains.NewRepository(db)
	domainHandler := domains.NewHandler(domainRepo)
	domainHandler.RegisterRoutes(webV1, domains.HandlerDeps{AuthMiddleware: authMw})

	server.RegisterDashboardRoutes(router)

	return router
}

func buildSchedulerDeps(db *gorm.DB) (*scraper.Service, *reporting.AggregationRepository) {
	eventsRepo := events.NewRepository(db)
	scrapeSvc := scraper.NewService(eventsRepo)
	aggRepo := reporting.NewAggregationRepository(db)
	return scrapeSvc, aggRepo
}
