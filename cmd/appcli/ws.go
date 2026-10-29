package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

// writeWait mirrors the server's write deadline: a wedged socket must
// not block the simulator forever.
const writeWait = 5 * time.Second

// Hello is the greeting the server sends right after the upgrade.
type Hello struct {
	DeviceID   uint64
	ServerTime int64
}

// PushEvent is a notification delivered over the WebSocket.
type PushEvent struct {
	PushID     uint64
	MessageID  string
	Category   pb.PushCategory
	Title      string
	Body       string
	ImageURL   string
	Priority   pb.PushPriority
	TTLSeconds int32
	Data       map[string]string
	SentAt     int64
}

// WSHandlers receives the frames the server sends. OnPush may return
// an error to stop listening.
type WSHandlers struct {
	OnHello func(Hello)
	OnPush  func(PushEvent) error
	// NoAck leaves the push unanswered, which is how a device that
	// is not delivering notifications looks to the backend.
	NoAck bool
}

// WSClient is the app-side WebSocket client. It mirrors what the
// Flutter client does: authenticate with the device token, ack every
// push and answer the server's keepalive pings.
type WSClient struct {
	url    string
	token  string
	dialer *websocket.Dialer
}

// NewWSClient returns a client for the app WebSocket endpoint.
func NewWSClient(baseURL, token string) *WSClient {
	endpoint := strings.TrimRight(baseURL, "/")
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		endpoint = "wss://" + strings.TrimPrefix(endpoint, "https://")
	case strings.HasPrefix(endpoint, "http://"):
		endpoint = "ws://" + strings.TrimPrefix(endpoint, "http://")
	}
	return &WSClient{
		url:    endpoint + appPathPrefix + "/ws",
		token:  token,
		dialer: &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
	}
}

// Listen connects to the WebSocket and blocks until the context is
// cancelled or the connection fails. Every push is acknowledged
// before the next frame is read, and every keepalive ping is
// answered with a pong.
func (w *WSClient) Listen(ctx context.Context, handlers WSHandlers) error {
	endpoint := w.url + "?token=" + url.QueryEscape(w.token)
	header := http.Header{}
	header.Set("APP-XIPTV", w.token)
	header.Set("User-Agent", AppUserAgent)

	conn, resp, err := w.dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket handshake failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	// Closing the socket is the only way to unblock a pending read
	// when the context is cancelled.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopped:
		}
	}()

	for {
		msgType, raw, err := conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			// The server closing the socket is a normal end of the
			// session, not a failure of the simulator.
			if websocket.IsCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure) {
				return nil
			}
			return fmt.Errorf("websocket read failed: %w", err)
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		frame := &pb.WsFrame{}
		if err := proto.Unmarshal(raw, frame); err != nil {
			fmt.Printf("appcli: frame descartado (proto inválido): %v\n", err)
			continue
		}

		switch {
		case frame.GetHello() != nil:
			if handlers.OnHello != nil {
				handlers.OnHello(Hello{
					DeviceID:   frame.GetHello().DeviceId,
					ServerTime: frame.GetHello().ServerTime,
				})
			}
		case frame.GetPush() != nil:
			push := frame.GetPush()
			if handlers.OnPush != nil {
				if err := handlers.OnPush(PushEvent{
					PushID:     push.PushId,
					MessageID:  push.MessageId,
					Category:   push.Category,
					Title:      push.Title,
					Body:       push.Body,
					ImageURL:   push.ImageUrl,
					Priority:   push.Priority,
					TTLSeconds: push.TtlSeconds,
					Data:       push.Data,
					SentAt:     push.SentAt,
				}); err != nil {
					return err
				}
			}
			if !handlers.NoAck {
				ack := &pb.WsFrame{Payload: &pb.WsFrame_PushAck{PushAck: &pb.WsPushAck{
					MessageId: push.MessageId,
					AckedAt:   time.Now().UnixMilli(),
				}}}
				if err := w.write(conn, ack); err != nil {
					return err
				}
			}
		case frame.GetPing() != nil:
			pong := &pb.WsFrame{Payload: &pb.WsFrame_Pong{Pong: &pb.WsPong{SentAt: time.Now().UnixMilli()}}}
			if err := w.write(conn, pong); err != nil {
				return err
			}
		case frame.GetError() != nil:
			return fmt.Errorf("server error %s: %s", frame.GetError().Code, frame.GetError().Message)
		}
	}
}

func (w *WSClient) write(conn *websocket.Conn, frame *pb.WsFrame) error {
	raw, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}
