package api

import (
	"net/http"

	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromToken(r)
	var req struct {
		Token    string `json:"token" cbor:"token"`
		Platform string `json:"platform" cbor:"platform"`
		Name     string `json:"name" cbor:"name"`
	}
	if err := decodeBody(r, &req); err != nil || req.Token == "" {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	device, err := repository.RegisterDevice(userID, req.Token, req.Platform, req.Name)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusOK, device)
}
