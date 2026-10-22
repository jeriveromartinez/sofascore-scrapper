package apk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TestClearWriteDeadline_LetsSlowHandlerRespond covers the streaming
// endpoints: Go sets the response write deadline when the request
// headers are read, so a handler that needs more than the server write
// timeout would lose its response even though the work succeeded.
func TestClearWriteDeadline_LetsSlowHandlerRespond(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.PUT("/apk/uploads/:id/chunks/:index", clearWriteDeadline, func(c *gin.Context) {
		time.Sleep(150 * time.Millisecond)
		c.String(http.StatusOK, "ok")
	})

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()

	req, err := http.NewRequest(http.MethodPut, server.URL+"/apk/uploads/"+uuid.NewString()+"/chunks/0", strings.NewReader("chunk"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chunk upload did not get a response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestUploadRoutes_SurviveExpiredWriteDeadline checks that the upload
// routes are actually wired to clear the write deadline: with an
// already expired deadline the server cannot deliver any response, so
// a client that gets one proves the deadline was cleared.
func TestUploadRoutes_SurviveExpiredWriteDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uploadID := uuid.New()
	service := newTestUploadService(t, uploadID, 7)
	handler := NewUploadHandler(service, service.store)

	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/web/v1"), AdminHandlerDeps{
		AuthMiddleware: func(c *gin.Context) { c.Set("userID", uint(7)) },
	})

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = time.Nanosecond
	server.Start()
	defer server.Close()

	req, err := http.NewRequest(http.MethodPut, server.URL+"/api/web/v1/apk/uploads/"+uploadID.String()+"/chunks/0", strings.NewReader("chunk"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chunk upload did not get a response: %v", err)
	}
	defer resp.Body.Close()
}
