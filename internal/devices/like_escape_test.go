package devices

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder keeps every executed statement so the tests can pin the
// LIKE escape character the repository sends to the database.
type sqlRecorder struct {
	mu    sync.Mutex
	stmts []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.stmts = append(r.stmts, sql)
	r.mu.Unlock()
}

func (r *sqlRecorder) containing(substr string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, s := range r.stmts {
		if strings.Contains(s, substr) {
			out = append(out, s)
		}
	}
	return out
}

func (r *sqlRecorder) reset() {
	r.mu.Lock()
	r.stmts = nil
	r.mu.Unlock()
}

func TestRegisterUsesPortableLikeEscape(t *testing.T) {
	db := newDevicesTestDB(t)
	rec := &sqlRecorder{}
	tracked := db.Session(&gorm.Session{Logger: rec})

	const host = "itest-escape.example.test"
	if err := tracked.Create(&apk.ApkVersion{
		Version:       "1.0",
		FileName:      "app.apk",
		FilePath:      "/tmp/app.apk",
		DownloadToken: "itest-escape-token",
		PackageName:   "com.example.escape",
		IPTVUrl:       "http://" + host,
	}).Error; err != nil {
		t.Fatalf("seed apk: %v", err)
	}
	// A subdomain ensures the LIKE fallback runs even when an exact
	// domain match is attempted first.
	domain := domains.Domain{Domain: "panel." + host}
	if err := tracked.Create(&domain).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	rec.reset()
	device, err := NewRepository(tracked).Register(context.Background(), nil, 0, "itest-escape-device", "android", "Box", "1.0", "com.example.escape", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if device.DomainID == nil || *device.DomainID != domain.ID {
		t.Fatalf("device domain: want %d, got %v", domain.ID, device.DomainID)
	}

	stmts := rec.containing("LIKE")
	if len(stmts) == 0 {
		t.Fatal("no LIKE query captured")
	}
	for _, stmt := range stmts {
		if !strings.Contains(stmt, "ESCAPE '!'") {
			t.Fatalf("LIKE query not portable to MariaDB (want ESCAPE '!'): %s", stmt)
		}
		if strings.Contains(stmt, "ESCAPE '\\'") {
			t.Fatalf("LIKE query still uses the backslash escape character: %s", stmt)
		}
	}
}

func TestEscapeLikeUsesBangAsEscapeChar(t *testing.T) {
	cases := map[string]string{
		"%":       "!%",
		"_":       "!_",
		"!":       "!!",
		`\`:       `\`,
		"a%b_c!d": "a!%b!_c!!d",
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
