package push

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var ackLoggingTestCounter int64

// TestOnAckReportsFailedPersistence asserts a failed ack write is
// reported through slog. The realtime ack handler cannot return an
// error, so the log is the only trace of a delivery that was never
// marked as delivered.
func TestOnAckReportsFailedPersistence(t *testing.T) {
	id := atomic.AddInt64(&ackLoggingTestCounter, 1)
	dsn := fmt.Sprintf("file:test_ack_logging_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The delivery_attempts table is missing on purpose: the
	// persistence of the ack fails.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := NewService(NewRepository(db), newFakePusher(), &fakeDomainRepo{}, &fakeUserRepo{}, logger)

	const messageID = "message-1"
	if err := svc.OnAck(context.Background(), messageID); err == nil {
		t.Fatal("expected the ack to fail")
	}

	out := logs.String()
	if !strings.Contains(out, "ack persistence failed") {
		t.Fatalf("the failed ack was not reported: %s", out)
	}
	if !strings.Contains(out, messageID) {
		t.Fatalf("the log does not identify the message: %s", out)
	}
}
