package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type logRecords struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logRecords) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// find returns the first structured record whose message contains msg.
func (l *logRecords) find(t *testing.T, msg string) map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	scanner := bufio.NewScanner(bytes.NewReader(l.buf.Bytes()))
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("decode log record %q: %v", scanner.Text(), err)
		}
		if m, _ := rec["msg"].(string); strings.Contains(m, msg) {
			return rec
		}
	}
	t.Fatalf("no log record containing %q in:\n%s", msg, l.buf.String())
	return nil
}

// captureSlog routes slog.Default to a JSON buffer for the test. Output
// written through the log package also lands here, but only as an INFO
// record without attributes, which is what the assertions tell apart.
func captureSlog(t *testing.T) *logRecords {
	t.Helper()
	records := &logRecords{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(records, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return records
}

func assertStructuredWarn(t *testing.T, rec map[string]any, attr string) {
	t.Helper()
	if rec["level"] != "WARN" {
		t.Fatalf("record level = %v, want WARN (record %v)", rec["level"], rec)
	}
	if _, ok := rec[attr]; !ok {
		t.Fatalf("record is missing structured attribute %q: %v", attr, rec)
	}
}

type failingLookup struct{}

func (failingLookup) TeamLogoURL(context.Context, string) (string, error) {
	return "", errors.New("lookup unavailable")
}

func TestDownloadAndPersistLogoLogsThroughSlog(t *testing.T) {
	records := captureSlog(t)
	repo := NewRepository(nil).WithLogoLookup(failingLookup{})
	repo.idSourceURL = func(int64) string { return "" }

	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{TeamID: 77, TeamName: "Lookup FC"})

	assertStructuredWarn(t, records.find(t, "logo lookup"), "team_id")
	assertStructuredWarn(t, records.find(t, "no logo source"), "team_id")
}

func TestTruncateTournamentIDsLogsThroughSlog(t *testing.T) {
	records := captureSlog(t)
	ids := make([]uint, maxTournamentIDs+1)

	if got := truncateTournamentIDs(9, ids); len(got) != maxTournamentIDs {
		t.Fatalf("len = %d, want %d", len(got), maxTournamentIDs)
	}
	assertStructuredWarn(t, records.find(t, "truncating"), "device_id")
}

type mapCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (c *mapCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	return v, ok, nil
}

func (c *mapCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	return nil
}

// newCachedService wires a Service over sqlite, an in-memory cache and
// a miniredis epoch store holding a live epoch, so requests take the
// cached path. It returns the cache key the first device request uses.
func newCachedService(t *testing.T) (*Service, *mapCache, string) {
	t.Helper()
	cache := &mapCache{data: map[string][]byte{}}
	svc, key := newCachedServiceWithCache(t, cache)
	return svc, cache, key
}

// eventsSlogDBCounter hands out the name of the in-memory database each
// call gets. A shared-cache database survives until its last connection
// closes, so the name has to be unique per call.
var eventsSlogDBCounter atomic.Int64

// newCachedServiceWithCache wires a Service over sqlite and a miniredis
// epoch store holding a live epoch, so requests take the cached path.
func newCachedServiceWithCache(t *testing.T, cache CurrentEventsCache) (*Service, string) {
	t.Helper()
	// The name is unique per call so a database left behind by an earlier
	// run cannot be picked up again, and the pool is closed so the database
	// does not outlive the test.
	dsn := fmt.Sprintf("file:events-slog-%d?mode=memory&cache=shared", eventsSlogDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1}).Error; err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	const epoch = int64(1)
	if err := client.Set(context.Background(), epochKey, epoch, 0).Err(); err != nil {
		t.Fatalf("seed epoch: %v", err)
	}

	return NewService(NewRepository(db), cache, NewEpochStore(client)), BuildCacheKey(epoch, []uint{1}, 6)
}

// TestCachedServiceUsesAFreshDatabasePerCall pins the hermeticity of the
// in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one seeded.
func TestCachedServiceUsesAFreshDatabasePerCall(t *testing.T) {
	newCachedService(t)
	newCachedService(t)
}

func TestServiceCorruptCacheLogsThroughSlog(t *testing.T) {
	svc, cache, key := newCachedService(t)
	cache.data[key] = []byte("not json")
	records := captureSlog(t)

	if _, err := svc.GetCurrentAndUpcoming(context.Background(), 1, 6); err != nil {
		t.Fatalf("GetCurrentAndUpcoming: %v", err)
	}
	assertStructuredWarn(t, records.find(t, "corrupt cache"), "key")
}
