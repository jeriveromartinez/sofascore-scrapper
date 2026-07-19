package apk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestUploadPublicationMatchesMigrationWithoutSoftDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	statement := db.Session(&gorm.Session{DryRun: true}).Where("upload_id = ?", uuid.NewString()).First(&UploadPublication{}).Statement
	if strings.Contains(statement.SQL.String(), "deleted_at") {
		t.Fatalf("publication query references column absent from migration: %s", statement.SQL.String())
	}
}

func TestUploadServiceStatusRejectsOwnerMismatch(t *testing.T) {
	uploadID := uuid.New()
	service := newTestUploadService(t, uploadID, 42)

	_, err := service.Status(context.Background(), uploadID, 7)
	if err == nil || !strings.Contains(err.Error(), "owner mismatch") {
		t.Fatalf("Status error = %v, want owner mismatch", err)
	}
}

func TestUploadServicePutChunkRejectsOwnerMismatch(t *testing.T) {
	uploadID := uuid.New()
	service := newTestUploadService(t, uploadID, 42)

	_, err := service.PutChunk(context.Background(), uploadID, 0, 7, bytes.NewReader([]byte("chunk")))
	if err == nil || !strings.Contains(err.Error(), "owner mismatch") {
		t.Fatalf("PutChunk error = %v, want owner mismatch", err)
	}
}

func TestUploadHandlerStatusRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUploadHandler(nil, nil)
	router := gin.New()
	router.GET("/apk/uploads/:id", handler.handleStatus)

	req := httptest.NewRequest(http.MethodGet, "/apk/uploads/"+uuid.NewString(), http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestUploadHandlerPutChunkPassesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uploadID := uuid.New()
	service := newTestUploadService(t, uploadID, 42)
	handler := NewUploadHandler(service, service.store)
	router := gin.New()
	router.PUT("/apk/uploads/:id/chunks/:index", func(c *gin.Context) {
		c.Set("userID", uint(7))
		c.Next()
	}, handler.handlePutChunk)

	req := httptest.NewRequest(http.MethodPut, "/apk/uploads/"+uploadID.String()+"/chunks/0", strings.NewReader("chunk"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
	if !strings.Contains(w.Body.String(), "owner mismatch") {
		t.Fatalf("body = %q, want owner mismatch", w.Body.String())
	}
}

func newTestUploadService(t *testing.T, uploadID uuid.UUID, ownerID uint) *UploadService {
	t.Helper()

	session, err := json.Marshal(map[string]any{
		"user_id":         ownerID,
		"file_name":       "test.apk",
		"file_size":       5,
		"total_chunks":    1,
		"chunks_received": 0,
		"bytes_received":  0,
		"status":          string(StatusReceiving),
		"version":         "1.0.0",
		"description":     "",
		"created_at":      time.Now().Add(-time.Minute).UnixMilli(),
		"expires_at":      time.Now().Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go serveTestRedis(listener, string(session))
	client := goredis.NewClient(&goredis.Options{
		Addr:            listener.Addr().String(),
		Protocol:        2,
		DisableIdentity: true,
	})
	t.Cleanup(func() { client.Close() })

	return NewUploadService(
		NewUploadStateStore(client),
		NewChunkStore(t.TempDir()),
		nil,
		nil,
	)
}

func serveTestRedis(listener net.Listener, session string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			reader := bufio.NewReader(conn)
			for {
				command, err := readRESPCommand(reader)
				if err != nil {
					return
				}
				switch strings.ToLower(command[0]) {
				case "hello":
					io.WriteString(conn, "-ERR unknown command 'hello'\r\n")
				case "client":
					io.WriteString(conn, "+OK\r\n")
				case "get":
					fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(session), session)
				default:
					io.WriteString(conn, "-ERR unsupported test command\r\n")
				}
			}
		}()
	}
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array: %q", line)
	}
	count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}

	command := make([]string, count)
	for i := range command {
		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) < 3 || line[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk length: %q", line)
		}
		length, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		command[i] = string(buf[:length])
	}
	return command, nil
}
