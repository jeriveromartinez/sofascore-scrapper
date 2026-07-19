package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type AuthHandler struct {
	authRepo   *AuthRepository
	userRepo   *users.Repository
	tokens     *TokenService
	invitation *InvitationStore
}

func NewAuthHandler(authRepo *AuthRepository, userRepo *users.Repository, tokens *TokenService, invitation *InvitationStore) *AuthHandler {
	return &AuthHandler{authRepo: authRepo, userRepo: userRepo, tokens: tokens, invitation: invitation}
}

func (h *AuthHandler) RegisterAuthRoutes(group *gin.RouterGroup, rateLimitMw gin.HandlerFunc) {
	group.POST("/users/register", rateLimitMw, h.handleRegister)
	group.POST("/users/login", rateLimitMw, h.handleLogin)
	group.POST("/users/refresh", rateLimitMw, h.handleRefresh)
	group.POST("/users/logout", AuthMiddleware(h.tokens), rateLimitMw, h.handleLogout)
	group.POST("/users/invitations", AuthMiddleware(h.tokens), rateLimitMw, h.handleCreateInvitation)
}

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req pb.AuthRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		server.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	if req.InvitationToken == "" {
		server.RespondError(c, http.StatusBadRequest, "invitation token is required")
		return
	}
	if err := h.invitation.Consume(c.Request.Context(), req.InvitationToken); err != nil {
		if err == ErrInvalidInvitation || err == ErrInvitationExpired {
			server.RespondError(c, http.StatusBadRequest, "invalid invitation token")
			return
		}
		server.RespondError(c, http.StatusServiceUnavailable, "invitation service unavailable")
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

func (h *AuthHandler) handleCreateInvitation(c *gin.Context) {
	var req pb.CreateInvitationRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ttl := DefaultInvitationTTL
	if req.TtlSeconds != 0 {
		if req.TtlSeconds < int64(MinInvitationTTL.Seconds()) || req.TtlSeconds > int64(MaxInvitationTTL.Seconds()) {
			server.RespondError(c, http.StatusBadRequest, "ttl_seconds must be between 300 and 604800")
			return
		}
		ttl = time.Duration(req.TtlSeconds) * time.Second
	}

	token, expiresAt, err := h.invitation.Create(c.Request.Context(), ttl)
	if err != nil {
		server.RespondError(c, http.StatusServiceUnavailable, "invitation creation failed")
		return
	}

	server.RespondProto(c, http.StatusCreated, &pb.InvitationResponse{
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
	})
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

	claims, err := h.tokens.ParseRefreshToken(refreshToken)
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	userID, err := claims.UserID()
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "user not found")
		return
	}

	accessToken, newRefreshToken, newTokenID, expiresAt, err := h.tokens.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}

	if err := h.authRepo.RotateRefreshToken(c.Request.Context(), user.ID, claims.ID, newTokenID, expiresAt); err != nil {
		if err == ErrInvalidRefreshToken {
			server.RespondError(c, http.StatusUnauthorized, "invalid token")
			return
		}
		server.RespondError(c, http.StatusInternalServerError, "token refresh failed")
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.AuthResponse{
		Id:           uint32(user.ID),
		Email:        user.Email,
		Token:        accessToken,
		RefreshToken: newRefreshToken,
	})
}

func (h *AuthHandler) handleLogout(c *gin.Context) {
	userID, ok := GetUserID(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	refreshToken := c.GetHeader("X-Refresh-Token")
	if refreshToken != "" {
		claims, err := h.tokens.ParseRefreshToken(refreshToken)
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
	accessToken, refreshToken, tokenID, expiresAt, err := h.tokens.GenerateTokenPair(userID, email)
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
