package events

import (
	"context"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder is a gorm logger that keeps every executed statement so
// tests can assert on the number and shape of queries.
type sqlRecorder struct {
	mu   sync.Mutex
	stmt []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.stmt = append(r.stmt, sql)
	r.mu.Unlock()
}

func (r *sqlRecorder) matching(prefix, contains string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, s := range r.stmt {
		upper := strings.ToUpper(strings.TrimSpace(s))
		if strings.HasPrefix(upper, prefix) && strings.Contains(s, contains) {
			out = append(out, s)
		}
	}
	return out
}

func (r *sqlRecorder) reset() {
	r.mu.Lock()
	r.stmt = nil
	r.mu.Unlock()
}

func recordSQL(db *gorm.DB) (*gorm.DB, *sqlRecorder) {
	rec := &sqlRecorder{}
	return db.Session(&gorm.Session{Logger: rec}), rec
}
