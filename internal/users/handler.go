package users

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"gorm.io/gorm"
)

type HandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

const defaultPageLimit = 20

func (h *Handler) RegisterUserRoutes(group *gin.RouterGroup, deps HandlerDeps) {
	group.GET("/users", deps.AuthMiddleware, h.handleGetUsers)
	group.GET("/users/page", deps.AuthMiddleware, h.handleGetUsersPage)
	group.GET("/users/:id", deps.AuthMiddleware, h.handleGetUser)
	group.POST("/users", deps.AuthMiddleware, h.handleCreate)
	group.PUT("/users/:id", deps.AuthMiddleware, h.handleUpdate)
	group.DELETE("/users/:id", deps.AuthMiddleware, h.handleDelete)
}

func (h *Handler) handleGetUsersPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var email string
	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 2)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		email = keys[0]
		parsedID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	users, hasMore, err := h.repo.ListPage(c.Request.Context(), email, id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(users) > 0 {
		last := users[len(users)-1]
		nextCursor, err = pagination.Encode(last.Email, strconv.FormatUint(uint64(last.ID), 10))
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.UserPage{
		Data: UsersToProto(users),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
}

func (h *Handler) handleGetUsers(c *gin.Context) {
	users, err := h.repo.GetAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.UserList{Users: UsersToProto(users)})
}

func (h *Handler) handleGetUser(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	user, err := h.repo.GetByID(id)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "user not found")
		return
	}

	server.RespondProto(c, http.StatusOK, UserToProto(*user))
}

func (h *Handler) handleCreate(c *gin.Context) {
	var req pb.UserWriteRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		server.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.repo.Create(req.Email, req.Password)
	if err != nil {
		server.RespondError(c, http.StatusConflict, "could not create user")
		return
	}

	server.RespondProto(c, http.StatusCreated, UserToProto(*user))
}

func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.UserWriteRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" {
		server.RespondError(c, http.StatusBadRequest, "email is required")
		return
	}

	user, err := h.repo.Update(id, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "user not found")
			return
		}
		server.RespondError(c, http.StatusConflict, "could not update user")
		return
	}

	server.RespondProto(c, http.StatusOK, UserToProto(*user))
}

func (h *Handler) handleDelete(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "user not found")
			return
		}
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "user deleted"})
}
