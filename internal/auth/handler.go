package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type AuthHandler struct {
	authRepo  *AuthRepository
	userRepo  *users.Repository
}

func NewAuthHandler(authRepo *AuthRepository, userRepo *users.Repository) *AuthHandler {
	return &AuthHandler{authRepo: authRepo, userRepo: userRepo}
}

func (h *AuthHandler) RegisterAuthRoutes(group *gin.RouterGroup) {
	group.POST("/users/register", h.handleRegister)
	group.POST("/users/login", h.handleLogin)
	group.POST("/users/refresh", h.handleRefresh)
	group.POST("/users/logout", AuthMiddleware(), h.handleLogout)
}

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req pb.AuthRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		server.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := h.userRepo.Create(req.Email, req.Password)
	if err != nil {
		server.RespondError(c, http.StatusConflict, "could not create user")
		return
	}
	response, err := h.buildAuthResponse(user.ID, user.Email)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	server.RespondProto(c, http.StatusCreated, response)
}

func (h *AuthHandler) handleLogin(c *gin.Context) {
	var req pb.AuthRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		server.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := h.userRepo.GetByEmail(req.Email)
	if err != nil || !CheckPassword(user.Password, req.Password) {
		server.RespondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	response, err := h.buildAuthResponse(user.ID, user.Email)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	server.RespondProto(c, http.StatusOK, response)
}

func (h *AuthHandler) handleRefresh(c *gin.Context) {
	refreshToken, ok := ExtractBearerToken(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "missing token")
		return
	}

	claims, err := ParseRefreshToken(refreshToken)
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	userID, err := claims.UserID()
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	active, err := h.authRepo.IsRefreshTokenActive(userID, claims.ID)
	if err != nil || !active {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "user not found")
		return
	}

	if err := h.authRepo.RevokeRefreshToken(userID, claims.ID); err != nil {
		server.RespondError(c, http.StatusInternalServerError, "token refresh failed")
		return
	}

	response, err := h.buildAuthResponse(user.ID, user.Email)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	server.RespondProto(c, http.StatusOK, response)
}

func (h *AuthHandler) handleLogout(c *gin.Context) {
	userID, ok := GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	refreshToken := c.GetHeader("X-Refresh-Token")
	if refreshToken != "" {
		claims, err := ParseRefreshToken(refreshToken)
		if err == nil {
			refreshUserID, userErr := claims.UserID()
			if userErr == nil && refreshUserID == userID {
				if err := h.authRepo.RevokeRefreshToken(userID, claims.ID); err != nil {
					server.RespondError(c, http.StatusInternalServerError, "logout failed")
					return
				}
				server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "logout successful"})
				return
			}
		}
	}

	if err := h.authRepo.RevokeAllRefreshTokens(userID); err != nil {
		server.RespondError(c, http.StatusInternalServerError, "logout failed")
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "logout successful"})
}

func (h *AuthHandler) buildAuthResponse(userID uint, email string) (*pb.AuthResponse, error) {
	accessToken, refreshToken, tokenID, expiresAt, err := GenerateTokenPair(userID, email)
	if err != nil {
		return nil, err
	}

	if err := h.authRepo.SaveRefreshToken(userID, tokenID, expiresAt); err != nil {
		return nil, err
	}

	return &pb.AuthResponse{
		Id:           uint32(userID),
		Email:        email,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}
