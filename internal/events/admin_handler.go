package events

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"gorm.io/gorm"
)

type AdminHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type AdminHandler struct {
	db *gorm.DB
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

func (h *AdminHandler) RegisterRoutes(group *gin.RouterGroup, deps AdminHandlerDeps) {
	group.GET("/events", deps.AuthMiddleware, h.handleGetEvents)
}

func (h *AdminHandler) handleGetEvents(c *gin.Context) {
	date := c.Query("date")
	sport := c.Query("sport")
	page := 1
	limit := 10

	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			server.RespondError(c, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			server.RespondError(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	query := h.db.Model(&Event{})
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			start := t.Unix()
			end := t.Add(24 * time.Hour).Unix()
			query = query.Where("start_timestamp >= ? AND start_timestamp < ?", start, end)
		}
	} else {
		start := time.Now().Unix()
		query = query.Where("start_timestamp >= ?", start)
	}

	if sport != "" {
		query = query.Where("sport = ?", sport)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var events []Event
	if err := query.Offset((page - 1) * limit).Limit(limit).Preload("HomeTeamModel").Preload("AwayTeamModel").Preload("League").Order("start_timestamp ASC").Find(&events).Error; err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	server.RespondProto(c, http.StatusOK, &pb.EventsList{
		Data:       EventsToProto(events),
		Page:       int32(page),
		Limit:      int32(limit),
		Total:      total,
		TotalPages: int32(totalPages),
	})
}
