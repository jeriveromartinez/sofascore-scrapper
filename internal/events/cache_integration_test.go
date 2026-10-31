//go:build integration

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

type fakeCache struct {
	mu       sync.Mutex
	data     map[string][]byte
	getErr   error
	setErr   error
	getCalls int
	setCalls int
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string][]byte)}
}

func (c *fakeCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	val, ok := c.data[key]
	return val, ok, nil
}

func (c *fakeCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}
	c.data[key] = value
	return nil
}

// newTestEpochStore returns an EpochStore backed by an in-process
// Redis that already holds a non-zero epoch, plus the epoch value
// the cache keys are built from. The service bypasses the cache
// while the epoch is 0 (every device would otherwise share one
// key), so the cache tests need a live epoch to reach the cached
// path at all.
func newTestEpochStore(t *testing.T) (*EpochStore, int64) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	const epoch = int64(1)
	if err := client.Set(context.Background(), epochKey, epoch, 0).Err(); err != nil {
		t.Fatalf("seed epoch: %v", err)
	}
	return NewEpochStore(client), epoch
}

// eventsCacheDBCounter hands out the name of the in-memory database each
// call gets. A shared-cache database survives until its last connection
// closes, so the name has to be unique per call.
var eventsCacheDBCounter atomic.Int64

func setupCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:events-cache-%d?mode=memory&cache=shared", eventsCacheDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Event{}, &Team{}, &tournaments.Tournament{}, &tournaments.DeviceTournament{}, &tournaments.GlobalTournamentConfig{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestCacheKey_SameSelectionSameKey(t *testing.T) {
	ids := []uint{3, 1, 2}
	key1 := BuildCacheKey(0, ids, 6)
	key2 := BuildCacheKey(0, []uint{1, 2, 3}, 6)
	if key1 != key2 {
		t.Errorf("same sorted set should produce same key: %s != %s", key1, key2)
	}
}

func TestCacheKey_DifferentEpochDifferentKey(t *testing.T) {
	ids := []uint{1, 2}
	key1 := BuildCacheKey(1, ids, 6)
	key2 := BuildCacheKey(2, ids, 6)
	if key1 == key2 {
		t.Error("different epochs should produce different keys")
	}
}

func TestCacheKey_DifferentLimitDifferentKey(t *testing.T) {
	ids := []uint{1, 2}
	key1 := BuildCacheKey(0, ids, 3)
	key2 := BuildCacheKey(0, ids, 6)
	if key1 == key2 {
		t.Error("different limits should produce different keys")
	}
}

func TestCacheKey_DifferentSelectionDifferentKey(t *testing.T) {
	key1 := BuildCacheKey(0, []uint{1, 2}, 6)
	key2 := BuildCacheKey(0, []uint{3, 4}, 6)
	if key1 == key2 {
		t.Error("different tournament sets should produce different keys")
	}
}

func TestService_GetCurrentAndUpcoming_CacheHit(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 1, "inprogress", 1710000000000, 1710000000000)

	fake := newFakeCache()
	epochStore, epoch := newTestEpochStore(t)

	svc := NewService(repo, fake, epochStore)

	cached := []Event{{ExternalMatchId: "999", Sport: "from-cache"}}
	cachedData, _ := json.Marshal(cached)
	fake.data[BuildCacheKey(epoch, []uint{1}, 6)] = cachedData

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ExternalMatchId != "999" {
		t.Errorf("expected cached event, got externalMatchID=%s", events[0].ExternalMatchId)
	}
	if events[0].Sport != "from-cache" {
		t.Errorf("expected sport='from-cache', got %s", events[0].Sport)
	}
	if fake.getCalls == 0 {
		t.Error("cache Get should have been called")
	}
	if fake.setCalls > 0 {
		t.Error("cache Set should not have been called on hit")
	}
}

func TestService_GetCurrentAndUpcoming_CacheMissFills(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 1, "inprogress", 1710000000000, 1710000000000)

	fake := newFakeCache()
	epochStore, epoch := newTestEpochStore(t)

	svc := NewService(repo, fake, epochStore)

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ExternalMatchId != "1" {
		t.Errorf("expected DB event, got externalMatchID=%s", events[0].ExternalMatchId)
	}
	if fake.setCalls == 0 {
		t.Error("cache Set should have been called on miss")
	}

	key := BuildCacheKey(epoch, []uint{1}, 6)
	if _, ok := fake.data[key]; !ok {
		t.Error("cache should have been populated on miss")
	}
}

func TestService_GetCurrentAndUpcoming_CorruptCacheRepopulates(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 2, "inprogress", 1710000000000, 1710000000000)

	fake := newFakeCache()
	epochStore, epoch := newTestEpochStore(t)

	svc := NewService(repo, fake, epochStore)

	fake.data[BuildCacheKey(epoch, []uint{1}, 6)] = []byte("not-valid-json")

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ExternalMatchId != "2" {
		t.Errorf("expected DB event after corrupt cache, got externalMatchID=%s", events[0].ExternalMatchId)
	}

	key := BuildCacheKey(epoch, []uint{1}, 6)
	newData, ok := fake.data[key]
	if !ok {
		t.Error("cache should be repopulated after corrupt read")
	}
	if string(newData) == "not-valid-json" {
		t.Error("corrupt cache should have been overwritten")
	}
}

func TestService_GetCurrentAndUpcoming_RedisErrorFallsBackToDB(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 3, "inprogress", 1710000000000, 1710000000000)

	fake := newFakeCache()
	fake.getErr = errors.New("redis connection refused")
	epochStore, _ := newTestEpochStore(t)

	svc := NewService(repo, fake, epochStore)

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming should not fail on Redis error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event from DB fallback, got %d", len(events))
	}
	if events[0].ExternalMatchId != "3" {
		t.Errorf("expected DB event, got externalMatchID=%s", events[0].ExternalMatchId)
	}
}

func TestService_GetCurrentAndUpcoming_NoCacheFallsBackToDB(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 4, "inprogress", 1710000000000, 1710000000000)

	epochStore, _ := newTestEpochStore(t)
	svc := NewService(repo, nil, epochStore)

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ExternalMatchId != "4" {
		t.Errorf("expected DB event, got externalMatchID=%s", events[0].ExternalMatchId)
	}
}

func TestService_GetCurrentAndUpcoming_DeviceTournamentsPreferred(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	db.Create(&tournaments.DeviceTournament{DeviceID: 42, TournamentID: 2})

	createTestEvent(repo, 1, "inprogress", 1710000000000, 1710000000000)
	createTestEventWithLeague(repo, 2, 2, "inprogress", 1710000000000, 1710000000000)

	epochStore, _ := newTestEpochStore(t)
	svc := NewService(repo, nil, epochStore)

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 42, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event from device assignment, got %d", len(events))
	}
	if events[0].ExternalMatchId != "2" {
		t.Errorf("expected event from device's tournament, got externalMatchID=%s", events[0].ExternalMatchId)
	}
}

func TestService_GetCurrentAndUpcoming_CoalescesConcurrentCacheMisses(t *testing.T) {
	const callers = 8
	db := setupCacheTestDB(t)
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1}).Error; err != nil {
		t.Fatalf("seed tournament selection: %v", err)
	}
	if err := db.Create(&tournaments.Tournament{Model: gorm.Model{ID: 1}, Name: "Test League", Slug: "test-league"}).Error; err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	if err := db.Create(&Event{ExternalMatchId: "1", LeagueId: 1, StatusType: "inprogress"}).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}

	var eventQueries atomic.Int32
	queryStarted := make(chan struct{}, callers)
	releaseQueries := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseQueries) }) }
	defer release()
	if err := db.Callback().Query().Before("gorm:query").Register("test:block_event_query", func(tx *gorm.DB) {
		if tx.Statement.Table != "events" {
			return
		}
		eventQueries.Add(1)
		queryStarted <- struct{}{}
		<-releaseQueries
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	cache := newFakeCache()
	epochStore, _ := newTestEpochStore(t)
	service := NewService(NewRepository(db), cache, epochStore)
	start := make(chan struct{})
	type result struct {
		events []Event
		err    error
	}
	results := make(chan result, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			events, err := service.GetCurrentAndUpcoming(context.Background(), 10, 1)
			results <- result{events: events, err: err}
		}()
	}
	close(start)

	select {
	case <-queryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for database query")
	}
	time.Sleep(100 * time.Millisecond)
	release()

	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("caller %d failed: %v", i, result.err)
		}
		if len(result.events) != 1 {
			t.Fatalf("caller %d: want 1 event, got %d", i, len(result.events))
		}
	}

	if got := eventQueries.Load(); got != 1 {
		t.Fatalf("event database queries: want 1, got %d", got)
	}
	cache.mu.Lock()
	getCalls := cache.getCalls
	cache.mu.Unlock()
	if getCalls != 1 {
		t.Fatalf("cache gets: want only the in-flight worker to consult the cache, got %d", getCalls)
	}
}

func TestService_GetCurrentAndUpcoming_CanceledCallerDoesNotCancelSharedLoad(t *testing.T) {
	db := setupCacheTestDB(t)
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1}).Error; err != nil {
		t.Fatalf("seed tournament selection: %v", err)
	}
	if err := db.Create(&tournaments.Tournament{Model: gorm.Model{ID: 1}, Name: "Test League", Slug: "test-league"}).Error; err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	if err := db.Create(&Event{ExternalMatchId: "1", LeagueId: 1, StatusType: "inprogress"}).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}

	queryStarted := make(chan struct{}, 2)
	releaseQueries := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseQueries) }) }
	defer release()
	if err := db.Callback().Query().Before("gorm:query").Register("test:block_event_query", func(tx *gorm.DB) {
		if tx.Statement.Table != "events" {
			return
		}
		queryStarted <- struct{}{}
		<-releaseQueries
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	epochStore, _ := newTestEpochStore(t)
	service := NewService(NewRepository(db), newFakeCache(), epochStore)
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := service.GetCurrentAndUpcoming(leaderCtx, 10, 1)
		leaderResult <- err
	}()

	select {
	case <-queryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for leader database query")
	}

	followerResult := make(chan error, 1)
	go func() {
		events, err := service.GetCurrentAndUpcoming(context.Background(), 10, 1)
		if err == nil && len(events) != 1 {
			err = fmt.Errorf("want 1 event, got %d", len(events))
		}
		followerResult <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancelLeader()

	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error: want context canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled caller remained blocked on the shared load")
	}

	release()
	select {
	case err := <-followerResult:
		if err != nil {
			t.Fatalf("follower failed after leader cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shared load did not terminate")
	}
}

func createTestEventWithLeague(repo *Repository, sofaID int64, leagueID uint, statusType string, startTs int64, currentPeriodTs int64) error {
	tm := Team{TeamId: sofaID + 100, Name: "Team"}
	event := Event{
		ExternalMatchId:             strconv.FormatInt(sofaID, 10),
		Sport:                       "football",
		HomeScore:                   0,
		HomeTeamId:                  100 + sofaID,
		AwayScore:                   0,
		AwayTeamId:                  200 + sofaID,
		StartTimestamp:              startTs,
		CurrentPeriodStartTimestamp: currentPeriodTs,
		Slug:                        "test-event",
		LeagueId:                    leagueID,
		StatusType:                  statusType,
	}
	batch := ScrapeBatch{
		Teams: []Team{tm},
		Tournaments: []tournaments.Tournament{
			{Model: gorm.Model{ID: leagueID}, Name: "Test League", Slug: "test-league"},
		},
		Events: []Event{event},
	}
	return repo.UpsertScrapeBatch(context.Background(), batch, 500)
}
