package apk

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newMiniredisDownloadCounter(t *testing.T) (*downloadCounter, *dbRecorder) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	db := setupTestDB(t)
	if err := db.AutoMigrate(&downloadCounterFlush{}); err != nil {
		t.Fatalf("migrate flushes: %v", err)
	}
	recorded, rec := recordDB(db)
	return NewDownloadCounter(client, recorded).(*downloadCounter), rec
}

func TestDownloadCounterFlushWritesUseCallerContext(t *testing.T) {
	counter, rec := newMiniredisDownloadCounter(t)
	apk := ApkVersion{Version: "1.0.0", FileName: "a.apk", FilePath: "/tmp/a.apk", PackageName: "com.test.ctx", DownloadToken: "ctx-token"}
	if err := counter.db.Create(&apk).Error; err != nil {
		t.Fatalf("seed apk: %v", err)
	}
	if err := counter.Increment(context.Background(), apk.ID); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	rec.reset()

	if err := counter.Flush(markedContext()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	rec.assertMarked(t, append(rec.matching("INSERT", "download_counter_flushes"), rec.matching("UPDATE", "apk_versions")...))

	var after ApkVersion
	if err := counter.db.First(&after, apk.ID).Error; err != nil {
		t.Fatalf("load apk: %v", err)
	}
	if after.TotalDownloads != 1 {
		t.Fatalf("total_downloads = %d, want 1", after.TotalDownloads)
	}
}
