package auth

import (
"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
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

func (h *AuthHandler) RegisterAuthRoutes(group *gin.RouterGroup, rateLimitMw, adminMw gin.HandlerFunc) {
	group.POST("/users/register", rateLimitMw, h.handleRegister)
	group.POST("/users/login", rateLimitMw, h.handleLogin)
	group.POST("/users/refresh", rateLimitMw, h.handleRefresh)
	group.POST("/users/logout", AuthMiddleware(h.tokens), rateLimitMw, h.handleLogout)
	group.POST("/users/invitations", AuthMiddleware(h.tokens), adminMw, rateLimitMw, h.handleCreateInvitation)
}

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req pb.AuthRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		server.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	if err := ValidatePassword(req.Password); err != nil {
		server.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.InvitationToken == "" {
		server.RespondError(c, http.StatusBadRequest, "invitation token is required")
		return
	}
	if err := h.invitation.Consume(c.Request.Context(), req.InvitationToken); err != nil {
		if errors.Is(err, ErrInvalidInvitation) || errors.Is(err, ErrInvitationExpired) {
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
	// The very first account bootstraps as admin so a fresh install has an
	// operator who can invite and manage others; subsequent users default to
	// the least-privileged role. The bootstrap invitation (see
	// runBootstrapInvitation) only issues while the users table is empty, so
	// this promotes exactly one account.
	if count, countErr := h.userRepo.Count(); countErr == nil && count == 1 {
		if promoted, roleErr := h.userRepo.SetRole(user.ID, users.RoleAdmin); roleErr == nil {
			user = promoted
		}
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
	// Normalize: trim surrounding whitespace from the email before
	// lookup. Password normalization is handled inside bcrypt and
	// the validator (which already trims).
	email := strings.TrimSpace(req.Email)
	user, err := h.userRepo.GetByEmail(email)
	if err != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// Lazy rehash: VerifyAndUpgrade re-hashes the stored password at
	// the current bcrypt cost (12) when the existing hash is below it.
	// The login is rejected only when the bcrypt comparison fails;
	// the upgrade is best-effort and logged.
	upgradedHash, verifyErr := VerifyAndUpgrade(user.Password, req.Password)
	if verifyErr != nil {
		server.RespondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if upgradedHash != "" {
		if updateErr := h.userRepo.UpdatePassword(user.ID, upgradedHash); updateErr != nil {
			slog.Default().Warn("auth: failed to upgrade password hash on login",
				slog.Uint64("user_id", uint64(user.ID)),
				slog.String("err", updateErr.Error()),
			)
		}
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
		if errors.Is(err, ErrInvalidRefreshToken) {
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

	// Targeted logout: caller must present a refresh token in the
	// X-Refresh-Token header. We only revoke the specific token that
	// matches the authenticated user.
	if refreshToken := c.GetHeader("X-Refresh-Token"); refreshToken != "" {
		claims, err := h.tokens.ParseRefreshToken(refreshToken)
		if err == nil {
			if refreshUserID, userErr := claims.UserID(); userErr == nil && refreshUserID == userID {
				if err := h.authRepo.RevokeRefreshToken(userID, claims.ID); err != nil {
					server.RespondError(c, http.StatusInternalServerError, "logout failed")
					return
				}
				server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "logout successful"})
				return
			}
		}
		// Header was present but unparseable, or did not match the
		// authenticated user — fall through to the 400 branch instead of
		// silently cascading into revoke-all. A stale tab without a
		// refresh token must not be able to wipe every other active
		// session for the same account.
		server.RespondError(c, http.StatusBadRequest, "missing or invalid X-Refresh-Token; pass ?all=true to revoke every session")
		return
	}

	// Bulk logout: explicit opt-in via ?all=true revokes every active
	// session for the user. Without this flag, refuse rather than guess.
	if c.Query("all") != "true" {
		server.RespondError(c, http.StatusBadRequest, "missing or invalid X-Refresh-Token; pass ?all=true to revoke every session")
		return
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
