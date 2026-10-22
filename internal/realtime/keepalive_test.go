package realtime

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

// newWSPair returns the server and client ends of a real WebSocket
// connection. It bypasses the HTTP handler so the keepalive and frame
// limit behaviour can be exercised without a database.
func newWSPair(t *testing.T) (server, client *websocket.Conn) {
	t.Helper()

	serverCh := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverCh <- conn
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	u.Scheme = "ws"
	client, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case server = <-serverCh:
	case <-time.After(5 * time.Second):
		t.Fatal("the server never completed the upgrade")
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, client
}

// TestKeepalive_ReapsHalfOpenConnection asserts that a client which
// stops answering the keepalive is dropped. The read deadline must be
// extended by the client's pong only: extending it on the server's own
// ping keeps half-open connections registered forever and every push
// routed to them is lost.
func TestKeepalive_ReapsHalfOpenConnection(t *testing.T) {
	serverConn, _ := newWSPair(t)

	conn := newConnection(1, 1, 1, serverConn, func(context.Context, string) {}, nil, nil)
	go writeLoop(conn, slog.Default(), 50*time.Millisecond)
	go readLoop(conn, slog.Default(), 300*time.Millisecond)

	select {
	case <-conn.Closed():
	case <-time.After(5 * time.Second):
		t.Fatal("the half-open connection was never reaped")
	}
}

// TestKeepalive_KeepsConnectionAliveWhenClientAnswers asserts that a
// client which answers the keepalive keeps the connection open.
func TestKeepalive_KeepsConnectionAliveWhenClientAnswers(t *testing.T) {
	serverConn, clientConn := newWSPair(t)

	conn := newConnection(1, 1, 1, serverConn, func(context.Context, string) {}, nil, nil)
	go writeLoop(conn, slog.Default(), 50*time.Millisecond)
	go readLoop(conn, slog.Default(), 400*time.Millisecond)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		pong := &pb.WsFrame{Payload: &pb.WsFrame_Pong{Pong: &pb.WsPong{SentAt: time.Now().UnixMilli()}}}
		raw, err := encodeFrame(pong)
		if err != nil {
			return
		}
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, _, err := clientConn.ReadMessage(); err != nil {
				return
			}
			_ = clientConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err := clientConn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
				return
			}
		}
	}()

	time.Sleep(1500 * time.Millisecond)
	if conn.IsClosed() {
		t.Fatal("the connection was closed even though the client answered the keepalive")
	}
}

// TestReadLoop_RejectsOversizedFrame asserts that a client cannot make
// the server buffer an unbounded frame.
func TestReadLoop_RejectsOversizedFrame(t *testing.T) {
	serverConn, clientConn := newWSPair(t)

	conn := newConnection(1, 1, 1, serverConn, func(context.Context, string) {}, nil, nil)
	go readLoop(conn, slog.Default(), 10*time.Second)

	payload := make([]byte, maxFrameBytes+1024)
	_ = clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := clientConn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("write oversized frame: %v", err)
	}

	select {
	case <-conn.Closed():
	case <-time.After(5 * time.Second):
		t.Fatal("the oversized frame was accepted")
	}
}
