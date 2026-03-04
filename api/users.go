package api

import (
	"net/http"

	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type UserController struct{
	Mux *http.ServeMux
}

func (c *UserController) LoadRoutes() {
	c.Mux.HandleFunc("/api/v1/users/register", handleRegister)
	c.Mux.HandleFunc("/api/v1/users/login", handleLogin)
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" cbor:"username"`
		Email    string `json:"email" cbor:"email"`
		Password string `json:"password" cbor:"password"`
	}
	if err := decodeBody(r, &req); err != nil || req.Username == "" || req.Email == "" || req.Password == "" {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "username, email, and password are required"})
		return
	}
	user, err := repository.CreateUser(req.Username, req.Email, req.Password)
	if err != nil {
		writeCBOR(w, http.StatusConflict, map[string]string{"error": "could not create user"})
		return
	}
	token, err := generateToken(user.ID, user.Username)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	writeCBOR(w, http.StatusCreated, map[string]any{"id": user.ID, "username": user.Username, "token": token})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" cbor:"username"`
		Password string `json:"password" cbor:"password"`
	}
	if err := decodeBody(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	user, err := repository.GetUserByUsername(req.Username)
	if err != nil || !repository.CheckPassword(user, req.Password) {
		writeCBOR(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	token, err := generateToken(user.ID, user.Username)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	writeCBOR(w, http.StatusOK, map[string]any{"id": user.ID, "username": user.Username, "token": token})
}
