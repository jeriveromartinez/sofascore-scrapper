package apk

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ctxMarkerKey struct{}

// markedContext returns a context the DB recorder can recognise, so a
// test can prove a statement ran under the caller's context.
func markedContext() context.Context {
	return context.WithValue(context.Background(), ctxMarkerKey{}, true)
}

type dbStatement struct {
	sql    string
	marked bool
}

// dbRecorder is a gorm logger that keeps every executed statement and
// whether it carried the marked context.
type dbRecorder struct {
	mu    sync.Mutex
	stmts []dbStatement
}

func (r *dbRecorder) LogMode(logger.LogLevel) logger.Interface { return r }
func (r *dbRecorder) Info(context.Context, string, ...any)     {}
func (r *dbRecorder) Warn(context.Context, string, ...any)     {}
func (r *dbRecorder) Error(context.Context, string, ...any)    {}

func (r *dbRecorder) Trace(ctx context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	marked, _ := ctx.Value(ctxMarkerKey{}).(bool)
	r.mu.Lock()
	r.stmts = append(r.stmts, dbStatement{sql: sql, marked: marked})
	r.mu.Unlock()
}

func (r *dbRecorder) matching(prefix, contains string) []dbStatement {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []dbStatement
	for _, s := range r.stmts {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s.sql)), prefix) && strings.Contains(s.sql, contains) {
			out = append(out, s)
		}
	}
	return out
}

func (r *dbRecorder) reset() {
	r.mu.Lock()
	r.stmts = nil
	r.mu.Unlock()
}

func (r *dbRecorder) assertMarked(t *testing.T, stmts []dbStatement) {
	t.Helper()
	if len(stmts) == 0 {
		t.Fatal("no statements recorded")
	}
	for _, s := range stmts {
		if !s.marked {
			t.Errorf("statement ran without the caller context: %s", s.sql)
		}
	}
}

func recordDB(db *gorm.DB) (*gorm.DB, *dbRecorder) {
	rec := &dbRecorder{}
	return db.Session(&gorm.Session{Logger: rec}), rec
}
