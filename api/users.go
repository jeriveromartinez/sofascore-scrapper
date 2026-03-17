package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	if err := parseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		respondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := repository.CreateUser(req.Email, req.Password)
	if err != nil {
		respondError(c, http.StatusConflict, "could not create user")
		return
	}
	token, err := generateToken(user.ID, user.Email)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	respondProto(c, http.StatusCreated, &pb.AuthResponse{
		Id:    uint32(user.ID),
		Email: user.Email,
		Token: token,
	})
}

func handleLogin(c *gin.Context) {
	var req pb.AuthRequest
	if err := parseProtoBody(c, &req); err != nil || req.Email == "" || req.Password == "" {
		respondError(c, http.StatusBadRequest, "email and password are required")
		return
	}
	user, err := repository.GetUserByEmail(req.Email)
	if err != nil || !repository.CheckPassword(user, req.Password) {
		respondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := generateToken(user.ID, user.Email)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "token generation failed")
		return
	}
	respondProto(c, http.StatusOK, &pb.AuthResponse{
		Id:    uint32(user.ID),
		Email: user.Email,
		Token: token,
	})
}
