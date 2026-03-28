package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type ReportController struct {
	Group *gin.RouterGroup
}

func (c *ReportController) LoadRoutes() {
	c.Group.POST("/crash-report", crashReport)
}

func crashReport(c *gin.Context) {
	var report models.CrashReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := repository.SaveCrashReport(report); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save crash report"})
		return
	}

	c.JSON(200, gin.H{"message": "Crash report saved successfully"})
}
