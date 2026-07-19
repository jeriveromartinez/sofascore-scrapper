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

func TestBodyLimitUploadRoutes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		limit  int64
	}{
		{name: "begin", method: http.MethodPost, path: "/api/web/v1/apk/uploads", limit: protobufBodyLimit},
		{name: "status", method: http.MethodGet, path: "/api/web/v1/apk/uploads/37cdb4a3-d02b-44d8-8e10-318876e20f19", limit: protobufBodyLimit},
		{name: "chunk", method: http.MethodPut, path: "/api/web/v1/apk/uploads/37cdb4a3-d02b-44d8-8e10-318876e20f19/chunks/0", limit: chunkUploadLimit},
		{name: "complete", method: http.MethodPost, path: "/api/web/v1/apk/uploads/37cdb4a3-d02b-44d8-8e10-318876e20f19/complete", limit: protobufBodyLimit},
		{name: "abort", method: http.MethodDelete, path: "/api/web/v1/apk/uploads/37cdb4a3-d02b-44d8-8e10-318876e20f19", limit: protobufBodyLimit},
		{name: "legacy direct", method: http.MethodPost, path: "/api/web/v1/apk/upload", limit: directUploadLimit},
		{name: "legacy chunk", method: http.MethodPost, path: "/api/web/v1/apk/upload/chunk", limit: chunkUploadLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(BodyLimit())
			router.Handle(tt.method, tt.path, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			for _, size := range []int64{tt.limit, tt.limit + 1} {
				req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
				req.ContentLength = size
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				want := http.StatusOK
				if size > tt.limit {
					want = http.StatusRequestEntityTooLarge
				}
				if w.Code != want {
					t.Fatalf("Content-Length %d: got status %d, want %d", size, w.Code, want)
				}
			}
		})
	}
}
