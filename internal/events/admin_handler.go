package events

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
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
	group.GET("/events/page", deps.AuthMiddleware, h.handleGetEventsPage)
}

const defaultPageLimit = 20

func (h *AdminHandler) handleGetEventsPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	direction := c.DefaultQuery("direction", "asc")
	if direction != "asc" && direction != "desc" {
		server.RespondError(c, http.StatusBadRequest, "direction must be 'asc' or 'desc'")
		return
	}
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var startTimestamp int64
	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 2)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		parsedTS, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad timestamp")
			return
		}
		startTimestamp = parsedTS
		parsedID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	events, hasMore, err := NewRepository(h.db).ListPage(c.Request.Context(), EventsPageFilter{
		CursorStartTimestamp: startTimestamp,
		CursorID:             id,
		Limit:                limit,
		Direction:            direction,
	})
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		nextCursor, err = pagination.Encode(
			strconv.FormatInt(last.StartTimestamp, 10),
			strconv.FormatUint(uint64(last.ID), 10),
		)
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.EventPage{
		Data: EventsToProto(events),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
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

	query := h.db.WithContext(c.Request.Context()).Model(&Event{})
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "date must use YYYY-MM-DD format (UTC)")
			return
		}
		start := t.UnixMilli()
		end := t.Add(24 * time.Hour).UnixMilli()
		query = query.Where("start_timestamp >= ? AND start_timestamp < ?", start, end)
	} else {
		start := time.Now().UnixMilli()
		query = query.Where("start_timestamp >= ?", start)
	}

	if sport != "" {
		query = query.Where("sport = ?", sport)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		server.RespondError(c, http.StatusInternalServerError, "unable to count events")
		return
	}

	var events []Event
	if err := query.Offset((page - 1) * limit).Limit(limit).Preload("HomeTeamModel").Preload("AwayTeamModel").Preload("League").Order("start_timestamp ASC").Find(&events).Error; err != nil {
		server.RespondError(c, http.StatusInternalServerError, "unable to fetch events")
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
