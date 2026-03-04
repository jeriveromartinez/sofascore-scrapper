package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	var req struct {
		Username string `json:"username" cbor:"username"`
		Email    string `json:"email" cbor:"email"`
		Password string `json:"password" cbor:"password"`
	}
	if err := bindBody(c, &req); err != nil || req.Username == "" || req.Email == "" || req.Password == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "username, email, and password are required"})
		return
	}
	user, err := repository.CreateUser(req.Username, req.Email, req.Password)
	if err != nil {
		respondCBOR(c, http.StatusConflict, map[string]string{"error": "could not create user"})
		return
	}
	token, err := generateToken(user.ID, user.Username)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	respondCBOR(c, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username, "token": token})
}

func handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" cbor:"username"`
		Password string `json:"password" cbor:"password"`
	}
	if err := bindBody(c, &req); err != nil || req.Username == "" || req.Password == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	user, err := repository.GetUserByUsername(req.Username)
	if err != nil || !repository.CheckPassword(user, req.Password) {
		respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	token, err := generateToken(user.ID, user.Username)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username, "token": token})
}
