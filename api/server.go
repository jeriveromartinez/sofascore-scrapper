package api

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/app"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/api/web"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func NewRouter() *gin.Engine {
	router := gin.New()
	router.Use(common.CorsMiddleware(), gin.Logger(), gin.Recovery())

	appV1 := router.Group("/api/app/v1")
	webV1 := router.Group("/api/web/v1")

	db, err := database.GetDB()
	if err != nil {
		log.Printf("warning: database not available, some routes will not function: %v", err)
	}

	apkRepo := apk.NewRepository(db)
	apkAppHandler := apk.NewAppHandler(apkRepo)
	apkAppHandler.RegisterRoutes(appV1)

	apkAdminHandler := apk.NewAdminHandler(apkRepo)
	apkAdminHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: common.AuthMiddleware()})

	devRepo := devices.NewRepository(db)
	playbackRepo := playback.NewRepository(db)

	playbackAppHandler := playback.NewAppHandler(playbackRepo, devRepo)
	playbackAppHandler.RegisterRoutes(appV1)

	playbackAdminHandler := playback.NewAdminHandler(playbackRepo)
	playbackAdminHandler.RegisterRoutes(webV1, playback.AdminHandlerDeps{AuthMiddleware: common.AuthMiddleware()})

	reportingRepo := reporting.NewRepository(db)
	crashHandler := reporting.NewCrashHandler(reportingRepo)
	crashHandler.RegisterRoutes(appV1)

	statsHandler := reporting.NewStatsHandler(reportingRepo)
	statsHandler.RegisterRoutes(webV1, reporting.StatsHandlerDeps{AuthMiddleware: common.AuthMiddleware()})

	eventsRepo := events.NewRepository(db)
	eventsAppHandler := events.NewAppHandler(eventsRepo)
	eventsAppHandler.RegisterRoutes(appV1)

	eventsAdminHandler := events.NewAdminHandler(db)
	eventsAdminHandler.RegisterRoutes(webV1, events.AdminHandlerDeps{AuthMiddleware: common.AuthMiddleware()})

	logoHandler := events.NewLogoHandler()
	logoHandler.RegisterRoutes(appV1)

	(&app.DeviceRegistrationController{Group: appV1}).LoadRoutes()

	(&web.UserController{Group: webV1}).LoadRoutes()
	(&web.DeviceController{Group: webV1}).LoadRoutes()
	(&web.TournamentController{Group: webV1}).LoadRoutes()
	(&web.DeviceTournamentController{Group: webV1}).LoadRoutes()
	(&web.GlobalConfigController{Group: webV1}).LoadRoutes()
	(&web.DomainController{Group: webV1}).LoadRoutes()

	web.RegisterDashboardRoutes(router)

	return router
}

func Start(addr string) {
	log.Printf("API server listening on %s", addr)
	if err := NewRouter().Run(addr); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}
