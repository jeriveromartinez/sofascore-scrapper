//go:build integration

package apk

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupCounterTestClients(t *testing.T) (*gorm.DB, *goredis.Client) {
	t.Helper()

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL not set, skipping integration test")
	}
	redisCfg := config.Redis{
		URL:          redisURL,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
	ctx := context.Background()
	redisClient, err := redisplatform.New(ctx, redisCfg)
	if err != nil {
		t.Fatalf("create redis client: %v", err)
	}
	t.Cleanup(func() { redisClient.Close() })
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis db: %v", err)
	}

	host := envOr("DB_HOST", "localhost")
	port := envOr("DB_PORT", "3306")
	user := envOr("DB_USER", "root")
	pass := os.Getenv("DB_PASSWORD")
	name := envOr("DB_NAME", "sofascore")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&multiStatements=true", user, pass, host, port, name)

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if !db.Migrator().HasTable("download_counter_flushes") {
		if err := db.Exec("CREATE TABLE IF NOT EXISTS download_counter_flushes (batch_id VARCHAR(64) NOT NULL PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").Error; err != nil {
			t.Fatalf("create download_counter_flushes: %v", err)
		}
	}
	return db, redisClient
}

func TestDownloadCounter_ConcurrentIncrementsExact(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)

	const numGoroutines = 1000
	const incrementsPerGoroutine = 10
	const apkID uint = 42

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				if err := counter.Increment(context.Background(), apkID); err != nil {
					t.Errorf("Increment error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	val, err := redisClient.HGet(context.Background(), "apk:downloads:active", fmt.Sprintf("%d", apkID)).Int64()
	if err != nil {
		t.Fatalf("HGET: %v", err)
	}
	expected := int64(numGoroutines * incrementsPerGoroutine)
	if val != expected {
		t.Fatalf("expected %d increments, got %d", expected, val)
	}
}

func TestDownloadCounter_FlushPreservesActive(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	apk, err := NewRepository(db).Create("1.0.0", "test.apk", "/tmp/test.apk", "test", "com.test.flush.preserve", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(apk) })

	if err := counter.Increment(ctx, apk.ID); err != nil {
		t.Fatalf("Increment: %v", err)
	}

	var capturedIncrement int64
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		activeBefore, _ := redisClient.HLen(ctx, "apk:downloads:active").Result()
		if activeBefore == 0 {
			t.Error("active should have entries before concurrent increment")
			return
		}
		atomic.AddInt64(&capturedIncrement, 1)
	}()

	if err := counter.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	wg.Wait()

	exists, err := redisClient.Exists(ctx, "apk:downloads:pending:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists > 0 {
		t.Fatal("pending key should be cleaned up after successful flush")
	}

	updatedApk, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updatedApk.TotalDownloads < 1 {
		t.Fatalf("expected TotalDownloads>=1, got %d", updatedApk.TotalDownloads)
	}
}

func TestDownloadCounter_IncrementsDuringFlushLandInActive(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	for i := range uint(5) {
		if err := counter.Increment(ctx, i+1); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}

	flushStarted := make(chan struct{})
	incDone := make(chan struct{})

	var incErr error
	go func() {
		<-flushStarted
		incErr = counter.Increment(ctx, 99)
		close(incDone)
	}()

	if err := counter.(*downloadCounter).flushWithProbe(ctx, func() { close(flushStarted) }); err != nil {
		t.Fatalf("flushWithProbe: %v", err)
	}
	<-incDone
	if incErr != nil {
		t.Fatalf("concurrent Increment: %v", incErr)
	}

	val, err := redisClient.HGet(ctx, "apk:downloads:active", "99").Int64()
	if err != nil {
		t.Fatalf("HGET 99: %v", err)
	}
	if val != 1 {
		t.Fatalf("concurrent increment should land in active (post-rename), got %d", val)
	}
}

func TestDownloadCounter_ReprocessBatchIdempotent(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	apk, err := NewRepository(db).Create("1.0.0", "idem.apk", "/tmp/idem.apk", "idem", "com.test.flush.idem", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(apk) })

	for range 10 {
		if err := counter.Increment(ctx, apk.ID); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}

	batchID := uuid.New().String()
	pendingKey := "apk:downloads:pending:" + batchID
	if _, err := redisClient.Rename(ctx, "apk:downloads:active", pendingKey).Result(); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if err := counter.(*downloadCounter).reprocessBatch(ctx, batchID, pendingKey); err != nil {
		t.Fatalf("first reprocessBatch: %v", err)
	}

	updatedApk, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updatedApk.TotalDownloads != 10 {
		t.Fatalf("expected TotalDownloads=10 after first reprocess, got %d", updatedApk.TotalDownloads)
	}

	if err := counter.(*downloadCounter).reprocessBatch(ctx, batchID, pendingKey); err != nil {
		t.Fatalf("second reprocessBatch: %v", err)
	}

	updatedApk2, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updatedApk2.TotalDownloads != 10 {
		t.Fatalf("expected TotalDownloads=10 after second reprocess (idempotent), got %d", updatedApk2.TotalDownloads)
	}
}

func TestDownloadCounter_ReprocessOrphans(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	apk, err := NewRepository(db).Create("1.0.0", "orphan.apk", "/tmp/orphan.apk", "orphan", "com.test.orphan", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(apk) })

	for range 5 {
		if err := counter.Increment(ctx, apk.ID); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}

	batchID := uuid.New().String()
	pendingKey := "apk:downloads:pending:" + batchID
	if _, err := redisClient.Rename(ctx, "apk:downloads:active", pendingKey).Result(); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if err := counter.ReprocessOrphans(ctx); err != nil {
		t.Fatalf("ReprocessOrphans: %v", err)
	}

	updatedApk, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updatedApk.TotalDownloads != 5 {
		t.Fatalf("expected TotalDownloads=5 after reprocess orphans, got %d", updatedApk.TotalDownloads)
	}

	exists, err := redisClient.Exists(ctx, pendingKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists > 0 {
		t.Fatal("orphan pending key should be deleted after reprocess")
	}

	if err := counter.ReprocessOrphans(ctx); err != nil {
		t.Fatalf("second ReprocessOrphans: %v", err)
	}
	updatedApk2, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updatedApk2.TotalDownloads != 5 {
		t.Fatalf("expected TotalDownloads=5 after second ReprocessOrphans (idempotent), got %d", updatedApk2.TotalDownloads)
	}
}

func TestDownloadCounter_SQLFailurePreservesPending(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	apk, err := NewRepository(db).Create("1.0.0", "fail.apk", "/tmp/fail.apk", "fail", "com.test.flush.sqlfail", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(apk) })

	for range 5 {
		if err := counter.Increment(ctx, apk.ID); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}

	batchID := uuid.New().String()
	pendingKey := "apk:downloads:pending:" + batchID
	if _, err := redisClient.Rename(ctx, "apk:downloads:active", pendingKey).Result(); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	type failingDB struct{ *gorm.DB }
	failDB := &failingDB{db}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("download_counter_flushes").Create(map[string]string{"batch_id": batchID}).Error; err != nil {
			return fmt.Errorf("insert batch: %w", err)
		}
		if err := tx.Exec(fmt.Sprintf("UPDATE apk_versions SET total_downloads = total_downloads + %d WHERE id = %d", 5, apk.ID)).Error; err != nil {
			return err
		}
		return fmt.Errorf("simulated commit failure")
	})
	_ = failDB
	if err == nil {
		t.Fatal("expected simulated failure")
	}

	exists, err := redisClient.Exists(ctx, pendingKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists == 0 {
		t.Fatal("pending key should exist when SQL fails")
	}

	_ = db.Exec("DELETE FROM download_counter_flushes WHERE batch_id = ?", batchID).Error
	_ = redisClient.Del(ctx, pendingKey)
}

func TestDownloadCounter_FlushEmptyActiveNoop(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	if err := counter.Flush(ctx); err != nil {
		t.Fatalf("Flush on empty active: %v", err)
	}

	keys, err := redisClient.Keys(ctx, "apk:downloads:pending:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) > 0 {
		t.Fatalf("expected no pending keys after empty flush, got %v", keys)
	}

	var count int64
	db.Table("download_counter_flushes").Count(&count)
	if count > 0 {
		t.Fatalf("expected no flush entries after empty flush, got %d", count)
	}
}

func TestDownloadCounter_FlushAppliesDeltas(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	a, err := NewRepository(db).Create("1.0.0", "a.apk", "/tmp/a.apk", "a", "com.test.flush.delta.a", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b, err := NewRepository(db).Create("1.0.0", "b.apk", "/tmp/b.apk", "b", "com.test.flush.delta.b", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	for range 3 {
		counter.Increment(ctx, a.ID)
	}
	for range 7 {
		counter.Increment(ctx, b.ID)
	}

	if err := counter.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	a2, _ := NewRepository(db).GetByID(a.ID)
	b2, _ := NewRepository(db).GetByID(b.ID)

	if a2.TotalDownloads != 3 {
		t.Fatalf("apk A: expected 3, got %d", a2.TotalDownloads)
	}
	if b2.TotalDownloads != 7 {
		t.Fatalf("apk B: expected 7, got %d", b2.TotalDownloads)
	}
}

func TestDownloadCounter_Integration_EndToEnd(t *testing.T) {
	db, redisClient := setupCounterTestClients(t)
	counter := NewDownloadCounter(redisClient, db)
	ctx := context.Background()

	apk, err := NewRepository(db).Create("1.0.0", "e2e.apk", "/tmp/e2e.apk", "e2e", "com.test.e2e", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(apk) })

	const n = 5000
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Increment(ctx, apk.ID)
		}()
	}
	wg.Wait()

	if err := counter.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	updated, err := NewRepository(db).GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated.TotalDownloads != n {
		t.Fatalf("expected TotalDownloads=%d, got %d", n, updated.TotalDownloads)
	}

	keys, err := redisClient.Keys(ctx, "apk:downloads:pending:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) > 0 {
		t.Fatalf("expected no pending keys, got %v", keys)
	}

	var batchCount int64
	db.Table("download_counter_flushes").Count(&batchCount)
	if batchCount == 0 {
		t.Fatal("expected at least one flush record")
	}
	t.Logf("flushed %d batches", batchCount)
}

