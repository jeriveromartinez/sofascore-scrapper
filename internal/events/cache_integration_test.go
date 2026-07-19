//go:build integration

package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

type fakeCache struct {
	data    map[string][]byte
	getErr  error
	setErr  error
	getCalls int
	setCalls int
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: make(map[string][]byte)}
}

func (c *fakeCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.getCalls++
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	val, ok := c.data[key]
	return val, ok, nil
}

func (c *fakeCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}
	c.data[key] = value
	return nil
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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
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
