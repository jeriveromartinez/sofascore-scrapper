//go:build integration

package reporting

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/playback"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mariadbDSN(database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		envDefault("DB_USER", "root"),
		os.Getenv("DB_PASSWORD"),
		envDefault("DB_HOST", "localhost"),
		envDefault("DB_PORT", "3306"),
		database,
	)
}

func openMariaDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// TestGenerateDaily_SumsSubSecondDurations runs the rollup against a
// scratch MariaDB schema. SUM() of integer millisecond columns returns
// DECIMAL there, so the query must not divide in SQL: a fractional
// result cannot be scanned into an integer field.
func TestGenerateDaily_SumsSubSecondDurations(t *testing.T) {
	admin := openMariaDB(t, mariadbDSN(""))

	scratch := fmt.Sprintf("%s_itest_%d", envDefault("DB_NAME", "scores365"), os.Getpid())
	if err := admin.Exec("CREATE DATABASE IF NOT EXISTS " + scratch).Error; err != nil {
		t.Skipf("cannot create scratch database %s: %v", scratch, err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE IF EXISTS " + scratch).Error; err != nil {
			t.Logf("drop database %s: %v", scratch, err)
		}
	})

	db := openMariaDB(t, mariadbDSN(scratch))
	if err := db.AutoMigrate(&playback.PlaybackLog{}, &ContentStat{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	now := time.Now()
	base := previousLocalMidnight(now).Add(time.Hour).UnixMilli()
	logs := []playback.PlaybackLog{
		{DeviceID: 1, Content: "itest-fractional", StartedAt: base + 1000, EndedAt: base + 31500},
		{DeviceID: 2, Content: "itest-fractional", StartedAt: base + 5000, EndedAt: base + 9500},
	}
	for i := range logs {
		if err := db.Create(&logs[i]).Error; err != nil {
			t.Fatalf("seed playback log: %v", err)
		}
	}

	if err := NewAggregationRepository(db).GenerateDaily(context.Background()); err != nil {
		t.Fatalf("GenerateDaily: %v", err)
	}

	var stat ContentStat
	if err := db.Where("content_hash = ? AND period_type = ?", "itest-fractional", PeriodTypeDay).First(&stat).Error; err != nil {
		t.Fatalf("daily stat not found: %v", err)
	}
	if stat.Seconds != 35 {
		t.Errorf("seconds = %d, want 35 (35000 ms)", stat.Seconds)
	}
	if stat.Views != 2 {
		t.Errorf("views = %d, want 2", stat.Views)
	}

	var remaining int64
	if err := db.Model(&playback.PlaybackLog{}).Where("content = ?", "itest-fractional").Count(&remaining).Error; err != nil {
		t.Fatalf("count playback logs: %v", err)
	}
	if remaining != 0 {
		t.Errorf("playback logs = %d, want 0 (rolled-up rows are deleted)", remaining)
	}
}
