package api

import (
	"net/http"
	"strconv"
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
	page := 1
	limit := 10

	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "page must be a positive integer"})
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

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

	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var events []models.SofaScoreEvent
	if err := query.Offset((page - 1) * limit).Limit(limit).Preload("HomeTeamModel").Preload("AwayTeamModel").Preload("League").Find(&events).Error; err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	respondCBOR(c, http.StatusOK, map[string]any{
		"data":      events,
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": totalPages,
	})
}
