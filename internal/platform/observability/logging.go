package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const (
	loggerKey    contextKey = "logger"
	requestIDKey contextKey = "requestID"
)

var sensitiveKeys = []string{
	"token", "access_token", "refresh_token", "accesstoken", "refreshtoken",
	"invitation", "password", "secret", "jwt", "app_xiptv", "device_token",
	"authorization", "x-refresh-token",
}

func NewLogger(level slog.Level, output io.Writer) *slog.Logger {
	handler := NewRedactHandler(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
	return slog.New(handler)
}

func WithRequest(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = uuid.New().String()
	}
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	logger := slog.Default().With(slog.String("request_id", requestID))
	ctx = context.WithValue(ctx, loggerKey, logger)
	return ctx
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

type RedactHandler struct {
	handler slog.Handler
}

func NewRedactHandler(handler slog.Handler) *RedactHandler {
	return &RedactHandler{handler: handler}
}

func (h *RedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error {
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		newRecord.AddAttrs(redactAttr(a))
		return true
	})
	return h.handler.Handle(ctx, newRecord)
}

func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &RedactHandler{handler: h.handler.WithAttrs(redacted)}
}

func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{handler: h.handler.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(key, sensitive) {
			return slog.String(a.Key, "[REDACTED]")
		}
	}
	return a
}
