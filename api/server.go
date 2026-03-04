package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	"github.com/golang-jwt/jwt/v5"
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
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return getJWTSecret(), nil
	})
	if err != nil || token == nil || !token.Valid {
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
