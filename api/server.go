package api

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/app"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/api/web"
)

func Start(addr string) {
	router := gin.New()
	router.Use(common.CorsMiddleware(), gin.Logger(), gin.Recovery())

	appV1 := router.Group("/api/app/v1")
	webV1 := router.Group("/api/web/v1")

	(&app.ApkController{Group: appV1}).LoadRoutes()
	(&app.CurrentEventsController{Group: appV1}).LoadRoutes()
	(&app.DeviceRegistrationController{Group: appV1}).LoadRoutes()
	(&app.TeamController{Group: appV1}).LoadRoutes()

	(&web.EventController{Group: webV1}).LoadRoutes()
	(&web.UserController{Group: webV1}).LoadRoutes()
	(&web.DeviceController{Group: webV1}).LoadRoutes()
	(&web.PlaybackController{Group: webV1}).LoadRoutes()
	(&web.StatsController{Group: webV1}).LoadRoutes()
	(&web.ApkController{Group: webV1}).LoadRoutes()
	(&web.TournamentController{Group: webV1}).LoadRoutes()
	(&web.DeviceTournamentController{Group: webV1}).LoadRoutes()
	(&web.GlobalConfigController{Group: webV1}).LoadRoutes()

	web.RegisterDashboardRoutes(router)

	log.Printf("API server listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}
