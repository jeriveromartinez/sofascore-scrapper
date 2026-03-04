package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

type EventController struct {
	Group *gin.RouterGroup
}

func (c *EventController) LoadRoutes() {
	c.Group.GET("/events", authMiddleware(), handleGetEvents)
}

func handleGetEvents(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	date := c.Query("date")
	sport := c.Query("sport")

	query := db.Model(&models.SofaScoreEvent{})
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			start := t.Unix()
			end := t.Add(24 * time.Hour).Unix()
			query = query.Where("start_timestamp >= ? AND start_timestamp < ?", start, end)
		}
	}
	if sport != "" {
		query = query.Where("sport = ?", sport)
	}

	var events []models.SofaScoreEvent
	query.Find(&events)
	respondCBOR(c, http.StatusOK, events)
}
