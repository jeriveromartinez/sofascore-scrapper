package web

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
	"gorm.io/gorm"
)

type UserController struct {
	Group *gin.RouterGroup
}

func (c *UserController) LoadRoutes() {
	c.Group.POST("/users/register", handleRegister)
	c.Group.POST("/users/login", handleLogin)
	c.Group.POST("/users/refresh", handleRefresh)
	c.Group.POST("/users/logout", common.AuthMiddleware(), handleLogout)
	c.Group.GET("/users", common.AuthMiddleware(), handleGetUsers)
	c.Group.GET("/users/:id", common.AuthMiddleware(), handleGetUser)
	c.Group.POST("/users", common.AuthMiddleware(), handleCreateManagedUser)
	c.Group.PUT("/users/:id", common.AuthMiddleware(), handleUpdateUser)
	c.Group.DELETE("/users/:id", common.AuthMiddleware(), handleDeleteUser)
}

func handleGetUsers(c *gin.Context) {
	users, err := repository.GetAllUsers()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.UserList{Users: common.UsersToProto(users)})
}

func handleGetUser(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	user, err := repository.GetUserByID(id)
	if err != nil {
		common.RespondError(c, http.StatusNotFound, "user not found")
		return
	}

	common.RespondProto(c, http.StatusOK, common.UserToProto(*user))
}

func handleCreateManagedUser(c *gin.Context) {
	var req pb.UserWriteRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		common.RespondError(c, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := repository.CreateUser(req.Email, req.Password)
	if err != nil {
		common.RespondError(c, http.StatusConflict, "could not create user")
		return
	}

	common.RespondProto(c, http.StatusCreated, common.UserToProto(*user))
}

func handleUpdateUser(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.UserWriteRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Email == "" {
		common.RespondError(c, http.StatusBadRequest, "email is required")
		return
	}

	user, err := repository.UpdateUser(id, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, http.StatusNotFound, "user not found")
			return
		}

		common.RespondError(c, http.StatusConflict, "could not update user")
		return
	}

	common.RespondProto(c, http.StatusOK, common.UserToProto(*user))
}

func handleDeleteUser(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := repository.DeleteUser(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, http.StatusNotFound, "user not found")
			return
		}

		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "user deleted"})
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
