//go:build integration

package users

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// openScratchMariaDB connects to a dedicated "<DB_NAME>_bootstrap"
// database: the bootstrap rule depends on an empty users table, which
// must never be arranged by wiping the configured database.
func openScratchMariaDB(t *testing.T) *gorm.DB {
	t.Helper()
	server := fmt.Sprintf("%s:%s@tcp(%s:%s)/",
		envOr("DB_USER", "root"), os.Getenv("DB_PASSWORD"), envOr("DB_HOST", "localhost"), envOr("DB_PORT", "3306"))
	admin, err := sql.Open("mysql", server)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	scratch := envOr("DB_NAME", "scores365") + "_bootstrap"
	if _, err := admin.Exec("CREATE DATABASE IF NOT EXISTS `" + scratch + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}

	conn, err := sql.Open("mysql", server+scratch+"?charset=utf8mb4&parseTime=True")
	if err != nil {
		t.Fatalf("sql.Open scratch: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := db.Migrator().DropTable(&User{}); err != nil {
		t.Fatalf("drop users: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	return db
}

func TestCreateAndMaybeBootstrapAdminConcurrentRegistrationsYieldOneAdmin_MariaDB(t *testing.T) {
	db := openScratchMariaDB(t)
	assertConcurrentBootstrapYieldsOneAdmin(t, db, NewRepository(db))
}
