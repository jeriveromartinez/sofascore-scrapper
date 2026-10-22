//go:build integration

package push

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func mariaDBDSN() string {
	env := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&multiStatements=true",
		env("DB_USER", "root"), os.Getenv("DB_PASSWORD"), env("DB_HOST", "localhost"),
		env("DB_PORT", "3306"), env("DB_NAME", "scores365"))
}

// newMariaDB opens the integration MariaDB and migrates the push
// schema. Skips when no database is reachable.
func newMariaDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("mysql", mariaDBDSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := db.AutoMigrate(
		&users.User{}, &domains.Domain{}, &devices.Device{},
		&PushMessage{}, &PushMessageTarget{},
		&ScheduledPush{}, &ScheduledPushTarget{}, &ScheduledPushTimer{},
		&DeliveryAttempt{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestBuildAggregateSnapshot_MariaDB(t *testing.T) {
	db := newMariaDB(t)
	assertAggregateSnapshot(t, db, seedAggregateFixture(t, db))
}
