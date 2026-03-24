package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type UserController struct {
	Group *gin.RouterGroup
}

func (c *UserController) LoadRoutes() {
	c.Group.POST("/users/register", handleRegister)
	c.Group.POST("/users/login", handleLogin)
	c.Group.POST("/users/refresh", handleRefresh)
	c.Group.POST("/users/logout", common.AuthMiddleware(), handleLogout)
}

func handleRegister(c *gin.Context) {
	var req pb.AuthRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		common.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := repository.CreateUser(req.Email, req.Password)
	if err != nil {
		common.RespondError(c, http.StatusConflict, "could not create user")
		return
	}
	response, err := buildAuthResponse(user.ID, user.Email)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	common.RespondProto(c, http.StatusCreated, response)
}

func handleLogin(c *gin.Context) {
	var req pb.AuthRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		common.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := repository.GetUserByEmail(req.Email)
	if err != nil || !repository.CheckPassword(user, req.Password) {
		common.RespondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	response, err := buildAuthResponse(user.ID, user.Email)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	common.RespondProto(c, http.StatusOK, response)
}

func handleRefresh(c *gin.Context) {
	refreshToken, ok := common.ExtractBearerToken(c)
	if !ok {
		common.RespondError(c, http.StatusUnauthorized, "missing token")
		return
	}

	claims, err := common.ParseRefreshToken(refreshToken)
	if err != nil {
		common.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	userID, err := claims.UserID()
	if err != nil {
		common.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	active, err := repository.IsRefreshTokenActive(userID, claims.ID)
	if err != nil || !active {
		common.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := repository.GetUserByID(userID)
	if err != nil {
		common.RespondError(c, http.StatusUnauthorized, "user not found")
		return
	}

	if err := repository.RevokeRefreshToken(userID, claims.ID); err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token refresh failed")
		return
	}

	response, err := buildAuthResponse(user.ID, user.Email)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	common.RespondProto(c, http.StatusOK, response)
}

func handleLogout(c *gin.Context) {
	userID, ok := common.GetUserID(c)
	if !ok {
		common.RespondError(c, http.StatusUnauthorized, "invalid token")
		return
	}

	refreshToken := c.GetHeader("X-Refresh-Token")
	if refreshToken != "" {
		claims, err := common.ParseRefreshToken(refreshToken)
		if err == nil {
			refreshUserID, userErr := claims.UserID()
			if userErr == nil && refreshUserID == userID {
				if err := repository.RevokeRefreshToken(userID, claims.ID); err != nil {
					common.RespondError(c, http.StatusInternalServerError, "logout failed")
					return
				}

				common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "logout successful"})
				return
			}
		}
	}

	if err := repository.RevokeAllRefreshTokens(userID); err != nil {
		common.RespondError(c, http.StatusInternalServerError, "logout failed")
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "logout successful"})
}

func buildAuthResponse(userID uint, email string) (*pb.AuthResponse, error) {
	accessToken, refreshToken, tokenID, expiresAt, err := common.GenerateTokenPair(userID, email)
	if err != nil {
		return nil, err
	}

	if err := repository.SaveRefreshToken(userID, tokenID, expiresAt); err != nil {
		return nil, err
	}

	return &pb.AuthResponse{
		Id:           uint32(userID),
		Email:        email,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}
