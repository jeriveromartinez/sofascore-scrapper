package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestWSClientAcksPushesAndAnswersPings(t *testing.T) {
	upgrader := websocket.Upgrader{}
	serverDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/ws", r.URL.Path)
		assert.Equal(t, "sim-token", r.URL.Query().Get("token"))

		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Hello{Hello: &pb.WsHello{DeviceId: 42, ServerTime: 1750000000}}})
		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
			PushId:    99,
			MessageId: "msg-1",
			Category:  pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
			Title:     "Gol",
			Body:      "Local 2 - 1 Visitante",
			Priority:  pb.PushPriority_PUSH_PRIORITY_HIGH,
			Data:      map[string]string{"match_id": "4711"},
			SentAt:    1750000000123,
		}}})

		// The client must ack the push with the same message_id.
		ack := readFrame(t, conn)
		require.NotNil(t, ack.GetPushAck())
		assert.Equal(t, "msg-1", ack.GetPushAck().MessageId)
		assert.Greater(t, ack.GetPushAck().AckedAt, int64(0))

		// The client must answer the keepalive ping with a pong.
		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Ping{Ping: &pb.WsPing{SentAt: 1750000001000}}})
		pong := readFrame(t, conn)
		require.NotNil(t, pong.GetPong())

		close(serverDone)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var gotHello Hello
	var gotPush PushEvent
	client := NewWSClient(srv.URL, "sim-token")

	err := client.Listen(ctx, WSHandlers{
		OnHello: func(h Hello) { gotHello = h },
		OnPush: func(p PushEvent) error {
			gotPush = p
			return nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, uint64(42), gotHello.DeviceID)
	assert.Equal(t, uint64(99), gotPush.PushID)
	assert.Equal(t, "msg-1", gotPush.MessageID)
	assert.Equal(t, "Gol", gotPush.Title)
	assert.Equal(t, "Local 2 - 1 Visitante", gotPush.Body)
	assert.Equal(t, pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT, gotPush.Category)
	assert.Equal(t, pb.PushPriority_PUSH_PRIORITY_HIGH, gotPush.Priority)
	assert.Equal(t, "4711", gotPush.Data["match_id"])

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func TestWSClientRejectsUnknownToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewWSClient(srv.URL, "bad-token")
	err := client.Listen(context.Background(), WSHandlers{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestWSClientStopsWhenContextIsCancelled(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Hello{Hello: &pb.WsHello{DeviceId: 1}}})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewWSClient(srv.URL, "sim-token")

	done := make(chan error, 1)
	go func() { done <- client.Listen(ctx, WSHandlers{}) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.True(t, err == nil || strings.Contains(err.Error(), "context canceled"), "unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Listen did not return after the context was cancelled")
	}
}

func writeFrame(t *testing.T, conn *websocket.Conn, frame *pb.WsFrame) {
	t.Helper()
	raw, err := proto.Marshal(frame)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, raw))
}

func readFrame(t *testing.T, conn *websocket.Conn) *pb.WsFrame {
	t.Helper()
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	frame := &pb.WsFrame{}
	require.NoError(t, proto.Unmarshal(raw, frame))
	return frame
}
