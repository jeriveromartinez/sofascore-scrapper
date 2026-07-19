package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLoggerJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("hello", slog.String("key", "value"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected JSON log line, got unmarshal error: %v\noutput: %s", err, buf.String())
	}
	if entry["msg"] != "hello" {
		t.Errorf("expected msg 'hello', got %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key 'value', got %v", entry["key"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected 'time' field in JSON log")
	}
}

func TestNewLoggerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelWarn, &buf)

	logger.Info("should not appear")
	logger.Warn("should appear")

	if strings.Contains(buf.String(), "should not appear") {
		t.Error("INFO message should be suppressed at WARN level")
	}
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("WARN message should appear at WARN level")
	}
}

func TestWithRequestPropagatesID(t *testing.T) {
	ctx := WithRequest(context.Background(), "test-id-123")

	id := RequestIDFromContext(ctx)
	if id != "test-id-123" {
		t.Errorf("expected request ID 'test-id-123', got '%s'", id)
	}

	logger := FromContext(ctx)
	if logger == nil {
		t.Fatal("expected non-nil logger from context")
	}
}

func TestWithRequestGeneratesIDWhenEmpty(t *testing.T) {
	ctx := WithRequest(context.Background(), "")

	id := RequestIDFromContext(ctx)
	if id == "" {
		t.Error("expected generated request ID, got empty string")
	}

	logger := FromContext(ctx)
	if logger == nil {
		t.Fatal("expected non-nil logger from context")
	}
}

func TestFromContextReturnsDefaultWhenEmpty(t *testing.T) {
	logger := FromContext(context.Background())
	if logger == nil {
		t.Fatal("expected non-nil default logger")
	}
}

func TestRedactAccessToken(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("auth", slog.String("access_token", "my-secret-token-abc"))

	output := buf.String()
	if strings.Contains(output, "my-secret-token-abc") {
		t.Error("access_token value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("access_token value should show [REDACTED]")
	}
}

func TestRedactRefreshToken(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("auth", slog.String("refresh_token", "my-refresh-secret-456"))

	output := buf.String()
	if strings.Contains(output, "my-refresh-secret-456") {
		t.Error("refresh_token value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("refresh_token value should show [REDACTED]")
	}
}

func TestRedactInvitation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("invite", slog.String("invitation", "invite-code-789"))

	output := buf.String()
	if strings.Contains(output, "invite-code-789") {
		t.Error("invitation value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("invitation value should show [REDACTED]")
	}
}

func TestRedactDeviceToken(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("device", slog.String("device_token", "device-secret-000"))
	logger.Info("header", slog.String("app_xiptv", "xtream-secret-111"))

	output := buf.String()
	if strings.Contains(output, "device-secret-000") {
		t.Error("device_token value should be redacted")
	}
	if strings.Contains(output, "xtream-secret-111") {
		t.Error("app_xiptv value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("redacted values should show [REDACTED]")
	}
}

func TestRedactPassword(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("login", slog.String("password", "my-password-123"))

	output := buf.String()
	if strings.Contains(output, "my-password-123") {
		t.Error("password value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("password value should show [REDACTED]")
	}
}

func TestRedactSecret(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("config", slog.String("jwt_secret", "super-secret-key"))

	output := buf.String()
	if strings.Contains(output, "super-secret-key") {
		t.Error("jwt_secret value should be redacted")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("jwt_secret value should show [REDACTED]")
	}
}

func TestNonSensitiveKeysAreNotRedacted(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	logger.Info("event", slog.String("sport", "football"), slog.Int("count", 42))

	output := buf.String()
	if !strings.Contains(output, "football") {
		t.Error("non-sensitive key 'sport' should not be redacted")
	}
	if strings.Contains(output, "count") && strings.Contains(output, `"42"`) {
		t.Error("integer values should not be affected")
	}
}

func TestRedactInNestedGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.LevelInfo, &buf)

	child := logger.With(slog.String("access_token", "nested-secret"))
	child.Info("nested")

	output := buf.String()
	if strings.Contains(output, "nested-secret") {
		t.Error("nested access_token in WithAttrs should be redacted")
	}
}

func TestRequestIDInLogOutput(t *testing.T) {
	var buf bytes.Buffer
	customLogger := NewLogger(slog.LevelInfo, &buf)
	prev := slog.Default()
	slog.SetDefault(customLogger)
	defer slog.SetDefault(prev)

	ctx := WithRequest(context.Background(), "req-abc-123")
	l := FromContext(ctx)
	l.Info("request received", slog.String("method", "GET"))

	output := buf.String()
	if !strings.Contains(output, "req-abc-123") {
		t.Error("log output should contain request ID")
	}
	if !strings.Contains(output, "request_id") {
		t.Error("log output should contain request_id field")
	}
}

func TestRequestIDNotRedacted(t *testing.T) {
	var buf bytes.Buffer
	customLogger := NewLogger(slog.LevelInfo, &buf)
	prev := slog.Default()
	slog.SetDefault(customLogger)
	defer slog.SetDefault(prev)

	ctx := WithRequest(context.Background(), "req-abc-123")
	l := FromContext(ctx)
	l.Info("check", slog.String("request_id", "req-abc-123"))

	output := buf.String()
	if !strings.Contains(output, "req-abc-123") {
		t.Error("request_id should not be redacted")
	}
}
