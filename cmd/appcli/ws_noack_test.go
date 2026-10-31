package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWSClientCanSkipAck(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
			PushId:    99,
			MessageId: "msg-1",
			Title:     "Gol",
		}}})

		// Without an ack the client must stay silent.
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(300*time.Millisecond)))
		_, _, err = conn.ReadMessage()
		assert.Error(t, err, "the client must not ack when NoAck is set")
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := make(chan PushEvent, 1)
	client := NewWSClient(srv.URL, "sim-token")

	err := client.Listen(ctx, WSHandlers{
		NoAck: true,
		OnPush: func(p PushEvent) error {
			received <- p
			return nil
		},
	})
	require.NoError(t, err)

	select {
	case push := <-received:
		assert.Equal(t, "msg-1", push.MessageID)
	default:
		t.Fatal("the push was never delivered")
	}
}
