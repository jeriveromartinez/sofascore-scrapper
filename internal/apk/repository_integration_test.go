//go:build integration

package apk

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func dbDSN() string {
	host := envOr("DB_HOST", "localhost")
	port := envOr("DB_PORT", "3306")
	user := envOr("DB_USER", "root")
	pass := os.Getenv("DB_PASSWORD")
	name := envOr("DB_NAME", "sofascore")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&multiStatements=true", user, pass, host, port, name)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func setupRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("mysql", dbDSN())
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
	ensureIntegrationSchema(t, sqlDB, db)
	return db
}

func ensureIntegrationSchema(t *testing.T, sqlDB *sql.DB, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasTable(&ApkVersion{}) {
		return
	}
	if err := db.AutoMigrate(&ApkVersion{}, &UploadPublication{}); err != nil {
		t.Fatalf("migrate integration database: %v", err)
	}
}

func TestGetLatest_ReturnsNewestVersion(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	a, err := repo.Create("1.0.9", "old.apk", "/tmp/old.apk", "old", "com.test.latest", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 1.0.9: %v", err)
	}
	b, err := repo.Create("1.0.10", "new.apk", "/tmp/new.apk", "new", "com.test.latest", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 1.0.10: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	latest, err := repo.GetLatest("com.test.latest")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.ID != b.ID {
		t.Errorf("expected latest ID %d (1.0.10), got %d (1.0.9)", b.ID, a.ID)
	}
}

func TestGetLatest_ExcludesInactive(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	active, err := repo.Create("1.0.0", "active.apk", "/tmp/active.apk", "active", "com.test.exclude", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create active: %v", err)
	}
	inactive, err := repo.Create("2.0.0", "inactive.apk", "/tmp/inactive.apk", "inactive", "com.test.exclude", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create inactive: %v", err)
	}
	db.Model(&ApkVersion{}).Where("id = ?", inactive.ID).Update("is_active", false)
	t.Cleanup(func() {
		db.Unscoped().Delete(active)
		db.Unscoped().Delete(inactive)
	})

	latest, err := repo.GetLatest("com.test.exclude")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.ID != active.ID {
		t.Errorf("expected active ID %d, got %d", active.ID, latest.ID)
	}
}

func TestGetLatest_PackageIsolation(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	a, err := repo.Create("1.0.0", "a.apk", "/tmp/a.apk", "a", "com.test.iso.a", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := repo.Create("2.0.0", "b.apk", "/tmp/b.apk", "b", "com.test.iso.b", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	latest, err := repo.GetLatest("com.test.iso.a")
	if err != nil {
		t.Fatalf("GetLatest A: %v", err)
	}
	if latest.PackageName != "com.test.iso.a" {
		t.Errorf("expected package com.test.iso.a, got %s", latest.PackageName)
	}
	if latest.ID != a.ID {
		t.Errorf("expected ID %d, got %d", a.ID, latest.ID)
	}
}

func TestCreate_RejectsDuplicateVersionForPackage(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	first, err := repo.Create("1.0.0", "first.apk", "/tmp/first.apk", "first", "com.test.tiebreak", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(first) })

	if _, err := repo.Create("1.0.0", "second.apk", "/tmp/second.apk", "second", "com.test.tiebreak", 200, 1, 21, 31); err == nil {
		t.Fatal("duplicate version and package should be rejected")
	}
}

func TestGetLatest_ExactlyOneRow(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	a, err := repo.Create("3.0.0", "a.apk", "/tmp/a.apk", "a", "com.test.single", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := repo.Create("1.0.0", "b.apk", "/tmp/b.apk", "b", "com.test.single", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	latest, err := repo.GetLatest("com.test.single")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatest returned nil")
	}
	if latest.PackageName != "com.test.single" {
		t.Errorf("wrong package: %s", latest.PackageName)
	}
}

func TestGetLatest_NoActiveVersion(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	_, err := repo.GetLatest("com.test.nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
}

func TestCreate_InvalidSemver(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	_, err := repo.Create("not.a.version", "bad.apk", "/tmp/bad.apk", "bad", "com.test.invalid", 100, 1, 21, 31)
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
}

func TestGetLatest_MajorVersionOrder(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	a, err := repo.Create("1.0.0", "a.apk", "/tmp/a.apk", "a", "com.test.major", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 1.0.0: %v", err)
	}
	b, err := repo.Create("10.0.0", "b.apk", "/tmp/b.apk", "b", "com.test.major", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 10.0.0: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	latest, err := repo.GetLatest("com.test.major")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.Version != "10.0.0" {
		t.Errorf("expected 10.0.0, got %s", latest.Version)
	}
}

func TestGetLatest_MinorVersionOrder(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewRepository(db)

	a, err := repo.Create("2.0.0", "a.apk", "/tmp/a.apk", "a", "com.test.minor", 100, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 2.0.0: %v", err)
	}
	b, err := repo.Create("2.9.0", "b.apk", "/tmp/b.apk", "b", "com.test.minor", 200, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create 2.9.0: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(a)
		db.Unscoped().Delete(b)
	})

	latest, err := repo.GetLatest("com.test.minor")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.Version != "2.9.0" {
		t.Errorf("expected 2.9.0, got %s", latest.Version)
	}
}
