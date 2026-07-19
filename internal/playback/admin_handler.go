package playback

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type AdminHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type AdminHandler struct {
	repo *Repository
}

func NewAdminHandler(repo *Repository) *AdminHandler {
	return &AdminHandler{repo: repo}
}

func (h *AdminHandler) RegisterRoutes(group *gin.RouterGroup, deps AdminHandlerDeps) {
	group.GET("/playback", deps.AuthMiddleware, h.handleGetPlayback)
	group.GET("/playback/page", deps.AuthMiddleware, h.handleGetPlaybackPage)
}

const defaultPageLimit = 20

func (h *AdminHandler) handleGetPlaybackPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var createdAtStr string
	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 2)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		createdAtStr = keys[0]
		parsedID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	logs, hasMore, err := h.repo.ListPage(c.Request.Context(), createdAtStr, id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(logs) > 0 {
		last := logs[len(logs)-1]
		nextCursor, err = pagination.Encode(
			last.CreatedAt.Format(time.RFC3339),
			strconv.FormatUint(uint64(last.ID), 10),
		)
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	result := make([]*pb.PlaybackLog, 0, len(logs))
	for i := range logs {
		result = append(result, PlaybackToProto(&logs[i]))
	}

	server.RespondProto(c, http.StatusOK, &pb.PlaybackPage{
		Data: result,
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
}

func (h *AdminHandler) handleGetPlayback(c *gin.Context) {
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

	logs, err := h.repo.GetList(page, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	total := h.repo.TotalCount()

	server.RespondProto(c, http.StatusOK, PlaybackListToProto(logs, total))
}
