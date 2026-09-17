package users

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/pagination"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
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
	group.PUT("/users/:id/role", deps.AuthMiddleware, h.handleSetRole)
	group.PUT("/users/:id/notifications", deps.AuthMiddleware, h.handleSetNotificationsEnabled)
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
	if err := ValidatePassword(req.Password); err != nil {
		server.RespondError(c, http.StatusBadRequest, err.Error())
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
	// Only validate the password if the caller is actually changing it.
	// An admin updating only the email should not be forced to re-submit
	// a strong password.
	if req.Password != "" {
		if err := ValidatePassword(req.Password); err != nil {
			server.RespondError(c, http.StatusBadRequest, err.Error())
			return
		}
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

func (h *Handler) handleSetRole(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.SetUserRoleRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	// The repository's SetRole performs the last-admin guard inside
	// a transaction with a re-count, so two concurrent demotions
	// cannot both pass the "more than one admin" check and lock
	// the operator out of the admin API. The handler just maps the
	// sentinel to HTTP 409.
	user, err := h.repo.SetRole(id, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRole):
			server.RespondError(c, http.StatusBadRequest, "role must be 'user' or 'admin'")
		case errors.Is(err, ErrLastAdmin):
			server.RespondError(c, http.StatusConflict, "cannot demote the last administrator")
		case errors.Is(err, gorm.ErrRecordNotFound):
			server.RespondError(c, http.StatusNotFound, "user not found")
		default:
			server.RespondError(c, http.StatusInternalServerError, "could not update role")
		}
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

// handleSetNotificationsEnabled flips the per-user push feature
// toggle. The body is a small SetNotificationsEnabledRequest proto
// (just a bool). The response is the updated User so the dashboard
// can refresh in one round trip.
//
// Authorization: today the route is mounted behind adminThenRl
// (admin or self) middleware in routes.go, so only an admin or the
// user themselves can reach this endpoint. The handler additionally
// re-checks ownership here so that swapping the middleware for a
// looser one in the future cannot escalate to flipping every other
// user's notification flag.
func (h *Handler) handleSetNotificationsEnabled(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	callerID, ok := callerUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if callerID != id {
		caller, err := h.repo.GetByID(callerID)
		if err != nil || !caller.IsAdmin() {
			server.RespondError(c, http.StatusForbidden, "cannot modify another user's notification settings")
			return
		}
	}

	var req pb.SetNotificationsEnabledRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.repo.SetNotificationsEnabled(id, req.Enabled)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "user not found")
			return
		}
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, UserToProto(*user))
}

// callerUserID returns the authenticated user's id from the gin
// context. It mirrors auth.GetUserID without importing the auth
// package (which would create an import cycle: auth already imports
// users). The key value MUST stay in sync with auth.tokens.userIDKey.
func callerUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get("userID")
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
