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
	// pongWait is the read deadline after a successful pong. If
	// the client misses two keepalives (ping every 30s), the
	// connection is dropped.
	pongWait = 60 * time.Second
	// pingPeriod must be smaller than pongWait; recommended 0.5x
	// or 0.6x. 30s gives two chances to pong before the deadline.
	pingPeriod = (pongWait * 9) / 10
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
		cfg.AckHandler = func(string) {}
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

		conn := newConnection(uint64(dev.ID), userIDPtrToUint32(dev.UserID), 0, ws, cfg.AckHandler, func(reason string) {
			cfg.Hub.Unregister(uint64(dev.ID))
			cfg.Logger.Info("realtime: connection closed",
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("reason", reason))
		}, cfg.Logger)
		if dev.DomainID != nil {
			conn.domainID = uint32(*dev.DomainID)
		}

		// Send the WsHello synchronously so the client knows the
		// upgrade completed before its first read. If this write
		// fails (network wedge, client tore down the socket right
		// after the upgrade handshake, etc.) the connection is
		// useless: register nothing, log the failure so it shows
		// up in dashboards, and let the socket be garbage-collected
		// by the underlying server. (Bug fix: B2 — previously the
		// error was discarded and a doomed connection was still
		// registered, which silently stranded every subsequent push.)
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

		cfg.Hub.Register(uint64(dev.ID), conn)
		cfg.Logger.Info("realtime: connection registered",
			slog.Uint64("device_id", uint64(dev.ID)),
			slog.String("platform", dev.Platform))

		go writeLoop(conn, cfg.Logger)
		go readLoop(conn, cfg.Logger)
	}
}

// writeLoop drains conn.send into the underlying socket. It exits
// when the send channel is closed (the connection's Close method
// does this) or when the socket fails.
func writeLoop(conn *Connection, logger *slog.Logger) {
	ticker := time.NewTicker(pingPeriod)
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
		}
	}
}

// readLoop pulls frames off the socket. Frames are binary proto;
// only the WsPushAck is meaningful for the server (the rest are
// ping/pong/keepalive which gorilla handles via the control
// channel). Any read error closes the connection.
func readLoop(conn *Connection, logger *slog.Logger) {
	// defer conn.Close(1000, "read loop exit")

	_ = conn.ws.SetReadDeadline(time.Now().Add(pongWait))
	conn.ws.SetPongHandler(func(string) error {
		_ = conn.ws.SetReadDeadline(time.Now().Add(pongWait))
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
			_ = conn.ws.SetReadDeadline(time.Now().Add(pongWait))
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

// userIDPtrToUint32 unwraps a *uint to uint32 with a zero default.
// Used because devices.UserID is nullable (the Flutter app may not
// have a user_id at registration time).
func userIDPtrToUint32(p *uint) uint32 {
	if p == nil {
		return 0
	}
	return uint32(*p)
}

// Compile-time guard so an unused import of "context" is avoided
// when the file is read in isolation by tooling.
var _ = context.Background
