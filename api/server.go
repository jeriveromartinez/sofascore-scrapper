package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func getJWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("WARNING: JWT_SECRET env var is not set; using insecure default. Set JWT_SECRET in production.")
	return []byte("changeme-please-set-JWT_SECRET-env")
}

func writeCBOR(w http.ResponseWriter, status int, v any) {
	data, err := cbor.Marshal(v)
	if err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(status)
	w.Write(data)
}

func decodeBody(r *http.Request, v any) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/cbor") {
		return cbor.NewDecoder(r.Body).Decode(v)
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func generateToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeCBOR(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return getJWTSecret(), nil
		})
		if err != nil || !token.Valid {
			writeCBOR(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		next(w, r)
	}
}

func getUserIDFromToken(r *http.Request) uint {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return getJWTSecret(), nil
	})
	if token == nil {
		return 0
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0
	}
	switch v := claims["sub"].(type) {
	case float64:
		return uint(v)
	case uint:
		return v
	}
	return 0
}

// --- Event handlers ---

func handleGetEvents(w http.ResponseWriter, r *http.Request) {
	db, err := database.GetDB()
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	date := r.URL.Query().Get("date")
	sport := r.URL.Query().Get("sport")

	query := db.Model(&models.SofaScoreEvent{})
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			start := t.Unix()
			end := t.Add(24 * time.Hour).Unix()
			query = query.Where("start_timestamp >= ? AND start_timestamp < ?", start, end)
		}
	}
	if sport != "" {
		query = query.Where("sport = ?", sport)
	}

	var events []models.SofaScoreEvent
	query.Find(&events)
	writeCBOR(w, http.StatusOK, events)
}

// --- User handlers ---

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
		writeCBOR(w, http.StatusConflict, map[string]string{"error": "user already exists"})
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

// --- Device handlers ---

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

// --- Playback handlers ---

func handleLogPlayback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceToken      string `json:"device_token" cbor:"device_token"`
		SofaScoreEventId int64  `json:"sofa_score_event_id" cbor:"sofa_score_event_id"`
		StartedAt        int64  `json:"started_at" cbor:"started_at"`
	}
	if err := decodeBody(r, &req); err != nil || req.SofaScoreEventId == 0 {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "sofa_score_event_id is required"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var device models.Device
	if err := db.Where("token = ?", req.DeviceToken).First(&device).Error; err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "device not found"})
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}
	playbackLog, err := repository.LogPlayback(device.ID, req.SofaScoreEventId, startedAt)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusCreated, playbackLog)
}

func handleUpdatePlayback(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/playback/")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		EndedAt int64 `json:"ended_at" cbor:"ended_at"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	endedAt := req.EndedAt
	if endedAt == 0 {
		endedAt = time.Now().Unix()
	}
	if err := repository.UpdatePlaybackEnd(uint(id), endedAt); err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusOK, map[string]string{"status": "updated"})
}

func handleTopEvents(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	stats, err := repository.GetTopEvents(limit)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusOK, stats)
}

// Start starts the HTTP API server.
func Start(addr string) {
	mux := http.NewServeMux()

	// Events
	mux.HandleFunc("/api/v1/events", handleGetEvents)

	// Users
	mux.HandleFunc("/api/v1/users/register", handleRegister)
	mux.HandleFunc("/api/v1/users/login", handleLogin)

	// Devices (requires auth)
	mux.HandleFunc("/api/v1/devices", authMiddleware(handleRegisterDevice))

	// Playback (requires auth)
	mux.HandleFunc("/api/v1/playback", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleLogPlayback(w, r)
		} else {
			writeCBOR(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))
	mux.HandleFunc("/api/v1/playback/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			handleUpdatePlayback(w, r)
		} else {
			writeCBOR(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))

	// Stats
	mux.HandleFunc("/api/v1/stats/top-events", handleTopEvents)

	log.Printf("API server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}
