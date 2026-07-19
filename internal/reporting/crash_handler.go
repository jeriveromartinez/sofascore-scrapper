package reporting

import (
	"github.com/gin-gonic/gin"
)

type CrashHandler struct {
	repo *Repository
}

func NewCrashHandler(repo *Repository) *CrashHandler {
	return &CrashHandler{repo: repo}
}

// RegisterRoutes mounts the crash-report endpoint. It is intentionally reachable
// without device authentication (an app may crash before it holds a session);
// abuse is bounded by the app-IP rate limit on the group and the global body
// size limit. See TestCrashReportInheritsAppIPRateLimit.
func (h *CrashHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/crash-report", h.handle)
}

func (h *CrashHandler) handle(c *gin.Context) {
	var report CrashReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(400, gin.H{"error": "invalid crash report"})
		return
	}

	if err := h.repo.SaveCrash(report); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save crash report"})
		return
	}

	c.JSON(200, gin.H{"message": "Crash report saved successfully"})
}
