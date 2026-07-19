package api

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/app"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/api/web"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
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

	(&app.CurrentEventsController{Group: appV1}).LoadRoutes()
	(&app.DeviceRegistrationController{Group: appV1}).LoadRoutes()
	(&app.TeamController{Group: appV1}).LoadRoutes()
	(&app.ReportController{Group: appV1}).LoadRoutes()

	(&web.EventController{Group: webV1}).LoadRoutes()
	(&web.UserController{Group: webV1}).LoadRoutes()
	(&web.DeviceController{Group: webV1}).LoadRoutes()
	(&web.PlaybackController{Group: webV1}).LoadRoutes()
	(&web.StatsController{Group: webV1}).LoadRoutes()
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
