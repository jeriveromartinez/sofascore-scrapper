package database

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type logRow struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

// loggerDB opens a SQLite database that reports through the production
// GORM logger, and returns the buffer the log records land in.
func loggerDB(t *testing.T) (*gorm.DB, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: newGormLogger(slog.New(slog.NewTextHandler(buf, nil))),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&logRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, buf
}

// TestGormLoggerIgnoresRecordNotFound covers the noise the stock
// logger produced: a lookup that misses is an expected outcome the
// application handles, not an error worth a log record.
func TestGormLoggerIgnoresRecordNotFound(t *testing.T) {
	db, buf := loggerDB(t)

	var row logRow
	err := db.First(&row, 42).Error
	if err == nil {
		t.Fatal("expected the lookup to miss")
	}

	if out := buf.String(); out != "" {
		t.Fatalf("a handled miss was logged: %s", out)
	}
}

// TestGormLoggerReportsRealErrors is the counterpart: an error the
// application cannot handle must still reach the log.
func TestGormLoggerReportsRealErrors(t *testing.T) {
	db, buf := loggerDB(t)

	if err := db.Exec("SELECT * FROM table_that_does_not_exist").Error; err == nil {
		t.Fatal("expected the statement to fail")
	}

	if out := buf.String(); !strings.Contains(out, "level=ERROR") {
		t.Fatalf("a real error was not logged: %s", out)
	}
}

// TestGormLoggerDoesNotInterpolateParameters covers the credential
// leak: the stock logger renders the trace SQL with the bound values
// substituted, so a device token or a password hash would be written
// to stdout on every error.
func TestGormLoggerDoesNotInterpolateParameters(t *testing.T) {
	db, buf := loggerDB(t)

	const secret = "s3cr3t-device-token"
	if err := db.Exec("SELECT * FROM table_that_does_not_exist WHERE name = ?", secret).Error; err == nil {
		t.Fatal("expected the statement to fail")
	}

	out := buf.String()
	if out == "" {
		t.Fatal("expected the failing statement to be logged")
	}
	if strings.Contains(out, secret) {
		t.Fatalf("the bound value reached the log: %s", out)
	}
}
