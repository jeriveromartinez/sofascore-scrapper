package web

import (
	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

func RegisterDashboardRoutes(router *gin.Engine) {
	server.RegisterDashboardRoutes(router)
}