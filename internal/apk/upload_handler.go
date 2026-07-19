package apk

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type UploadHandler struct {
	service *UploadService
	store   *UploadStateStore
}

func NewUploadHandler(service *UploadService, store *UploadStateStore) *UploadHandler {
	return &UploadHandler{service: service, store: store}
}

func (h *UploadHandler) RegisterRoutes(group *gin.RouterGroup, deps AdminHandlerDeps) {
	group.POST("/apk/uploads", deps.AuthMiddleware, h.handleBegin)
	group.GET("/apk/uploads/:id", deps.AuthMiddleware, h.handleStatus)
	group.PUT("/apk/uploads/:id/chunks/:index", deps.AuthMiddleware, h.handlePutChunk)
	group.POST("/apk/uploads/:id/complete", deps.AuthMiddleware, h.handleComplete)
	group.DELETE("/apk/uploads/:id", deps.AuthMiddleware, h.handleAbort)
}

func (h *UploadHandler) handleBegin(c *gin.Context) {
	var req pb.UploadBeginRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	userID, ok := auth.GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.Begin(c.Request.Context(), userID, &req)
	if err != nil {
		server.RespondError(c, http.StatusConflict, err.Error())
		return
	}

	server.RespondProto(c, http.StatusCreated, resp)
}

func (h *UploadHandler) handleStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid upload ID")
		return
	}

	userID, ok := auth.GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.Status(c.Request.Context(), id, userID)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, resp)
}

func (h *UploadHandler) handlePutChunk(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid upload ID")
		return
	}

	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || index < 0 {
		server.RespondError(c, http.StatusBadRequest, "invalid chunk index")
		return
	}

	userID, ok := auth.GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.PutChunk(c.Request.Context(), id, index, userID, c.Request.Body)
	if err != nil {
		server.RespondError(c, http.StatusConflict, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, resp)
}

func (h *UploadHandler) handleComplete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid upload ID")
		return
	}

	userID, ok := auth.GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.Complete(c.Request.Context(), id, userID)
	if err != nil {
		server.RespondError(c, http.StatusConflict, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, resp)
}

func (h *UploadHandler) handleAbort(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid upload ID")
		return
	}

	userID, ok := auth.GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.service.Abort(c.Request.Context(), id, userID); err != nil {
		server.RespondError(c, http.StatusConflict, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "aborted"})
}
