package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimitProtoAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit())
	router.POST("/api/web/v1/users/login", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"size": len(body)})
	})

	payload := bytes.Repeat([]byte("a"), 1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/users/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for 1 MiB payload, got %d", w.Code)
	}
}

func TestBodyLimitProtoRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit())
	router.POST("/api/web/v1/users/login", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.JSON(http.StatusOK, gin.H{})
	})

	payload := bytes.Repeat([]byte("a"), 1024*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/users/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for 1 MiB+1 payload, got %d", w.Code)
	}
}

func TestBodyLimitContentLengthRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit())
	router.POST("/api/web/v1/users/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	payload := make([]byte, 1024*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/users/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.ContentLength = int64(len(payload))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized Content-Length, got %d", w.Code)
	}
}

func TestBodyLimitUploadRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit())
	router.POST("/api/web/v1/apk/upload", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	payload := bytes.Repeat([]byte("a"), 1024*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/apk/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for upload route with 1 MiB+1 payload, got %d", w.Code)
	}
}

func TestBodyLimitChunkRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit())
	router.POST("/api/web/v1/apk/upload/chunk", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	payload := bytes.Repeat([]byte("a"), 1024*1024+1)
	req := httptest.NewRequest(http.MethodPost, "/api/web/v1/apk/upload/chunk", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for chunk upload route with 1 MiB+1 payload, got %d", w.Code)
	}
}
