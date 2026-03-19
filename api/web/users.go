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
	token, err := common.GenerateToken(user.ID, user.Email)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	common.RespondProto(c, http.StatusCreated, &pb.AuthResponse{
		Id:    uint32(user.ID),
		Email: user.Email,
		Token: token,
	})
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
	token, err := common.GenerateToken(user.ID, user.Email)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.AuthResponse{
		Id:    uint32(user.ID),
		Email: user.Email,
		Token: token,
	})
}
