//go:build integration

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
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

type missBarrierCache struct {
	cache   *fakeCache
	total   int
	mu      sync.Mutex
	arrived int
	release chan struct{}
}

func newMissBarrierCache(total int) *missBarrierCache {
	return &missBarrierCache{cache: newFakeCache(), total: total, release: make(chan struct{})}
}

func (c *missBarrierCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	c.arrived++
	if c.arrived == c.total {
		close(c.release)
	}
	release := c.release
	c.mu.Unlock()

	select {
	case <-release:
		return c.cache.Get(ctx, key)
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

func (c *missBarrierCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.cache.Set(ctx, key, value, ttl)
}

type fakeEpoch struct {
	val   int64
	calls int
}

func (e *fakeEpoch) Get(ctx context.Context) (int64, error) {
	e.calls++
	return e.val, nil
}

func (e *fakeEpoch) Increment(ctx context.Context) (int64, error) {
	e.val++
	return e.val, nil
}

func setupCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:events-cache-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(16)
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
	_ = &fakeEpoch{val: 0}

	svc := NewService(repo, fake, &EpochStore{client: nil})

	cached := []Event{{SofaScoreEventId: 999, Sport: "from-cache"}}
	cachedData, _ := json.Marshal(cached)
	fake.data[BuildCacheKey(0, []uint{1}, 6)] = cachedData

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 999 {
		t.Errorf("expected cached event, got sofaID=%d", events[0].SofaScoreEventId)
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

	svc := NewService(repo, fake, &EpochStore{client: nil})

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 1 {
		t.Errorf("expected DB event, got sofaID=%d", events[0].SofaScoreEventId)
	}
	if fake.setCalls == 0 {
		t.Error("cache Set should have been called on miss")
	}

	key := BuildCacheKey(0, []uint{1}, 6)
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

	svc := NewService(repo, fake, &EpochStore{client: nil})

	fake.data[BuildCacheKey(0, []uint{1}, 6)] = []byte("not-valid-json")

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 2 {
		t.Errorf("expected DB event after corrupt cache, got sofaID=%d", events[0].SofaScoreEventId)
	}

	key := BuildCacheKey(0, []uint{1}, 6)
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

	svc := NewService(repo, fake, &EpochStore{client: nil})

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming should not fail on Redis error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event from DB fallback, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 3 {
		t.Errorf("expected DB event, got sofaID=%d", events[0].SofaScoreEventId)
	}
}

func TestService_GetCurrentAndUpcoming_NoCacheFallsBackToDB(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	createTestEvent(repo, 4, "inprogress", 1710000000000, 1710000000000)

	svc := NewService(repo, nil, &EpochStore{client: nil})

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 0, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 4 {
		t.Errorf("expected DB event, got sofaID=%d", events[0].SofaScoreEventId)
	}
}

func TestService_GetCurrentAndUpcoming_DeviceTournamentsPreferred(t *testing.T) {
	db := setupCacheTestDB(t)
	repo := NewRepository(db)

	db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1})
	db.Create(&tournaments.DeviceTournament{DeviceID: 42, TournamentID: 2})

	createTestEvent(repo, 1, "inprogress", 1710000000000, 1710000000000)
	createTestEventWithLeague(repo, 2, 2, "inprogress", 1710000000000, 1710000000000)

	svc := NewService(repo, nil, &EpochStore{client: nil})

	events, err := svc.GetCurrentAndUpcoming(context.Background(), 42, 6)
	if err != nil {
		t.Fatalf("GetCurrentAndUpcoming failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event from device assignment, got %d", len(events))
	}
	if events[0].SofaScoreEventId != 2 {
		t.Errorf("expected event from device's tournament, got sofaID=%d", events[0].SofaScoreEventId)
	}
}

func TestService_GetCurrentAndUpcoming_CoalescesConcurrentCacheMisses(t *testing.T) {
	const callers = 8
	db := setupCacheTestDB(t)
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1}).Error; err != nil {
		t.Fatalf("seed tournament selection: %v", err)
	}
	if err := db.Create(&Event{SofaScoreEventId: 1, LeagueId: 1, StatusType: "inprogress"}).Error; err != nil {
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

	cache := newMissBarrierCache(callers)
	service := NewService(NewRepository(db), cache, &EpochStore{client: nil})
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
	cache.cache.mu.Lock()
	getCalls := cache.cache.getCalls
	cache.cache.mu.Unlock()
	if getCalls != callers+1 {
		t.Fatalf("cache gets: want %d outer checks plus one flight revalidation, got %d", callers+1, getCalls)
	}
}

func TestService_GetCurrentAndUpcoming_CanceledCallerDoesNotCancelSharedLoad(t *testing.T) {
	db := setupCacheTestDB(t)
	if err := db.Create(&tournaments.GlobalTournamentConfig{TournamentID: 1}).Error; err != nil {
		t.Fatalf("seed tournament selection: %v", err)
	}
	if err := db.Create(&Event{SofaScoreEventId: 1, LeagueId: 1, StatusType: "inprogress"}).Error; err != nil {
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

	service := NewService(NewRepository(db), newFakeCache(), &EpochStore{client: nil})
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
		SofaScoreEventId:            sofaID,
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
		HomeTeamModel:               &tm,
	}
	return repo.Upsert(context.Background(), []Event{event}, "football")
}
