//go:build integration

package realtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"gorm.io/gorm"
)

// handshakeOnlyConn accepts the upgrade response and fails every later
// write, like a client that tears the socket down right after the
// handshake.
type handshakeOnlyConn struct {
	net.Conn
	writes atomic.Int32
}

func (c *handshakeOnlyConn) Write(p []byte) (int, error) {
	if c.writes.Add(1) > 1 {
		return 0, errors.New("socket gone")
	}
	return len(p), nil
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
	conn net.Conn
}

func (w *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

func countGoroutines(fn string) int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	return strings.Count(string(buf), fn)
}

// wsHelloDBCounter hands out the name of the in-memory database each call
// gets. A shared-cache database survives until its last connection closes,
// so the name has to be unique per call.
var wsHelloDBCounter atomic.Int64

// TestWSHandler_FailedHelloDoesNotLeakConnectionGoroutines guards the
// leak on the WsHello failure path: the socket was closed without
// closing the Connection, so its ack worker blocked forever, one
// goroutine per failed handshake.
func TestWSHandler_FailedHelloDoesNotLeakConnectionGoroutines(t *testing.T) {
	dsn := fmt.Sprintf("file:test_ws_hello_%d?mode=memory&cache=shared", wsHelloDBCounter.Add(1))
	db, err := gorm.Open(openDriver(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// The pool is closed so the named database does not outlive the test.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&devices.Device{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	dev := &devices.Device{Token: "tok-hello", Platform: "android", Name: "Box", Version: "1.0"}
	if err := db.Create(dev).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}

	hub := NewHub()
	handler := Handler(HandlerConfig{
		Authenticator: NewAuthenticator(db),
		Hub:           hub,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		AckHandler:    func(context.Context, string) {},
	})

	const ackWorker = "realtime.(*Connection).ackLoop"
	baseline := countGoroutines(ackWorker)

	const attempts = 5
	for i := 0; i < attempts; i++ {
		serverSide, clientSide := net.Pipe()
		t.Cleanup(func() { _ = clientSide.Close() })

		req := httptest.NewRequest(http.MethodGet, "/api/app/v1/ws", nil)
		req.Header.Set(headerAppXIPTV, dev.Token)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		w := &hijackRecorder{ResponseRecorder: httptest.NewRecorder(), conn: &handshakeOnlyConn{Conn: serverSide}}
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		handler(c)
	}

	if n := hub.Count(); n != 0 {
		t.Fatalf("hub.Count = %d, want 0 (WsHello failed, nothing may be registered)", n)
	}

	deadline := time.Now().Add(2 * time.Second)
	for countGoroutines(ackWorker) > baseline && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if leaked := countGoroutines(ackWorker) - baseline; leaked > 0 {
		t.Fatalf("%d ack worker goroutines leaked after %d failed handshakes", leaked, attempts)
	}
}
