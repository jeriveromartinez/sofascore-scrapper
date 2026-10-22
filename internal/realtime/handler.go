package realtime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	// headerAppXIPTV is the existing app-side auth header used by
	// the REST API. The WS upgrade reuses the same token so the
	// Flutter client does not need a new secret.
	headerAppXIPTV = "APP-XIPTV"
	// queryToken is the fallback for clients that cannot set
	// headers (some embedded browsers, proxies). Same value as
	// the header; either is accepted.
	queryToken = "token"

	// writeWait caps the time a single WriteMessage may take. If
	// the underlying TCP socket is wedged, this fires and the
	// connection is closed with a write timeout error.
	writeWait = 5 * time.Second
	// pongWait is the read deadline after a successful keepalive
	// ack. A client that stops answering the keepalive is dropped.
	pongWait = 60 * time.Second
	// pingPeriod is the server keepalive cadence. It must stay well
	// below pongWait so the client has a full pongWait window to
	// answer before the connection is reaped.
	pingPeriod = 30 * time.Second
	// maxFrameBytes caps the size of a single inbound WebSocket
	// frame. The protocol only carries small control and push
	// envelopes, so anything bigger is a client bug or an attack.
	maxFrameBytes = 1 << 20
)

// HandlerConfig wires a Gin upgrade endpoint to the realtime stack.
// The defaults are sensible for a single-tenant deployment; tests
// can override individual fields.
type HandlerConfig struct {
	Authenticator *Authenticator
	Hub           *Hub
	Logger        *slog.Logger
	// AckHandler is called for every WsPushAck the client sends.
	// The push service installs a real implementation that flips
	// delivery_attempts to DELIVERED; the realtime package does
	// not depend on internal/push.
	AckHandler AckHandler
	// Upgrader defaults to gorilla's defaults. Tests may tighten
	// CheckOrigin or replace the dialer.
	Upgrader websocket.Upgrader
	// PingInterval overrides the keepalive cadence. Zero uses
	// pingPeriod.
	PingInterval time.Duration
	// PongTimeout overrides the read deadline applied after a
	// keepalive ack. Zero uses pongWait.
	PongTimeout time.Duration
}

// Handler returns a Gin handler that upgrades the request to a
// WebSocket, authenticates via APP-XIPTV (or ?token=), and
// registers the connection with the hub. The returned handler
// installs the read and write goroutines and wires the close
// cleanup back to the hub so the connection is automatically
// unregistered when the socket goes away.
func Handler(cfg HandlerConfig) gin.HandlerFunc {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Upgrader.CheckOrigin == nil {
		cfg.Upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	}
	if cfg.AckHandler == nil {
		cfg.AckHandler = func(context.Context, string) {}
	}
	pingInterval := cfg.PingInterval
	if pingInterval <= 0 {
		pingInterval = pingPeriod
	}
	pongTimeout := cfg.PongTimeout
	if pongTimeout <= 0 {
		pongTimeout = pongWait
	}

	return func(c *gin.Context) {
		token := c.GetHeader(headerAppXIPTV)
		if token == "" {
			token = c.Query(queryToken)
		}

		dev, err := cfg.Authenticator.AuthenticateToken(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, ErrInvalidToken) {
				cfg.Logger.Info("realtime: upgrade rejected, invalid token", slog.String("client_ip", c.ClientIP()))
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			cfg.Logger.Error("realtime: auth lookup failed", slog.String("error", err.Error()))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		ws, err := cfg.Upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			cfg.Logger.Warn("realtime: upgrade failed",
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			return
		}

		// Send the WsHello synchronously so the client knows the
		// upgrade completed before its first read. If this write
		// fails (network wedge, client tore down the socket right
		// after the upgrade handshake, etc.) the connection is
		// useless: register nothing, log the failure so it shows
		// up in dashboards, and close the socket. (Bug fix: B2 — previously the
		// error was discarded and a doomed connection was still
		// registered, which silently stranded every subsequent push.)
		// The Connection is only built once the hello is out: its ack
		// worker exits on Close, so creating it earlier leaked one
		// goroutine per failed handshake.
		hello := &pbWsHello{
			DeviceId:   uint64(dev.ID),
			ServerTime: time.Now().Unix(),
		}
		helloRaw, err := encodeFrame(hello.toProto())
		if err != nil {
			cfg.Logger.Warn("realtime: WsHello encode failed, not registering connection",
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			_ = ws.Close()
			return
		}
		_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
		if werr := ws.WriteMessage(websocket.BinaryMessage, helloRaw); werr != nil {
			cfg.Logger.Warn("realtime: WsHello write failed, not registering connection",
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", werr.Error()))
			_ = ws.Close()
			return
		}

		conn := newConnection(uint64(dev.ID), ws, cfg.AckHandler, func(conn *Connection, reason string) {
			cfg.Hub.UnregisterConn(uint64(dev.ID), conn)
			cfg.Logger.Info("realtime: connection closed",
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("reason", reason))
		}, cfg.Logger)

		cfg.Hub.Register(uint64(dev.ID), conn)
		cfg.Logger.Info("realtime: connection registered",
			slog.Uint64("device_id", uint64(dev.ID)),
			slog.String("platform", dev.Platform))

		go writeLoop(conn, cfg.Logger, pingInterval)
		go readLoop(conn, cfg.Logger, pongTimeout)
	}
}

// writeLoop drains conn.send into the underlying socket. It exits
// when the send channel is closed (the connection's Close method
// does this) or when the socket fails.
func writeLoop(conn *Connection, logger *slog.Logger, pingInterval time.Duration) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-conn.Closed():
			return
		case raw, ok := <-conn.send:
			if !ok {
				return
			}
			_ = conn.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.ws.WriteMessage(websocket.BinaryMessage, raw); err != nil {
				// Upgraded from Debug -> Warn: a write failure here
				// means a push was enqueued for a socket that turned
				// dead between enqueue and write. The push service has
				// already logged the push_id; this line ties it to the
				// device and the byte count for cross-referencing.
				logger.Warn("realtime: ws frame write failed",
					slog.Uint64("device_id", conn.DeviceID()),
					slog.Int("bytes", len(raw)),
					slog.String("error", err.Error()))
				conn.Close(1011, "write error")
				return
			}
		case <-ticker.C:
			_ = conn.ws.SetWriteDeadline(time.Now().Add(writeWait))
			pingFrame := (pbWsPing{SentAt: time.Now().UnixMilli()}).toProto()
			raw, err := encodeFrame(pingFrame)
			if err != nil {
				logger.Warn("realtime: ws ping encode failed",
					slog.Uint64("device_id", conn.DeviceID()),
					slog.String("error", err.Error()))
				continue
			}
			// Application frame (WsFrame with oneof=ping) instead of
			// PingMessage control frame: the Dart client (web_socket_channel
			// in IO mode) does not auto-handle control frames, and its
			// only way to reply is with a WsPong application frame.
			// Keeping the keepalive inside the proto envelope lets both
			// peers share a single framing model.
			if err := conn.ws.WriteMessage(websocket.BinaryMessage, raw); err != nil {
				logger.Warn("realtime: ws ping write failed",
					slog.Uint64("device_id", conn.DeviceID()),
					slog.String("error", err.Error()))
				conn.Close(1011, "ping write error")
				return
			}
			// The read deadline is deliberately NOT extended here. It is
			// the client's answer that proves the connection is alive:
			// extending it on our own writes would keep half-open
			// connections (client vanished without a FIN) registered
			// forever, and every push routed to them would be lost
			// silently.
		}
	}
}

// readLoop pulls frames off the socket. Frames are binary proto;
// only the WsPushAck is meaningful for the server (the rest are
// ping/pong/keepalive which gorilla handles via the control
// channel). Any read error closes the connection.
func readLoop(conn *Connection, logger *slog.Logger, pongTimeout time.Duration) {
	defer conn.Close(1000, "read loop exit")

	conn.ws.SetReadLimit(maxFrameBytes)
	_ = conn.ws.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.ws.SetPongHandler(func(string) error {
		_ = conn.ws.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		msgType, raw, err := conn.ws.ReadMessage()
		if err != nil {
			if !isExpectedClose(err) {
				logger.Debug("realtime: read error",
					slog.Uint64("device_id", conn.DeviceID()),
					slog.String("error", err.Error()))
			}
			conn.Close(1000, err.Error())
			return
		}
		// We only act on binary application frames. Text frames and
		// control frames are ignored (the pong handler above covers
		// control frames).
		if msgType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodeFrame(raw)
		if err != nil {
			logger.Debug("realtime: bad frame",
				slog.Uint64("device_id", conn.DeviceID()),
				slog.String("error", err.Error()))
			continue
		}
		if ack := frame.GetPushAck(); ack != nil {
			conn.onAck(ack.MessageId)
		}
		// WsPong is the application-frame counterpart to a WsPing. It
		// arrives as the keepalive ack from clients that do not
		// auto-handle WebSocket control frames (e.g. Dart
		// web_socket_channel in IO mode). Reset the read deadline so
		// the connection stays open.
		if frame.GetPong() != nil {
			_ = conn.ws.SetReadDeadline(time.Now().Add(pongTimeout))
		}
		// Other inbound frames (Hello/Push/Error) are not expected
		// from the client and are silently dropped.
	}
}

// isExpectedClose reports whether err is a benign WS close (the
// client navigated away, the app was backgrounded, etc.). We
// suppress the log for these because they are the normal case.
func isExpectedClose(err error) bool {
	return errors.Is(err, websocket.ErrCloseSent) ||
		websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseAbnormalClosure)
}

// Compile-time check that Handler is wired through gin.HandlerFunc.
var _ gin.HandlerFunc = Handler(HandlerConfig{})
