//go:build integration

package database

import (
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

func TestOpenAppliesPoolConfig(t *testing.T) {
	cfg := config.Database{
		Host:            envOr("DB_HOST", "localhost"),
		Port:            envOr("DB_PORT", "3306"),
		User:            envOr("DB_USER", "root"),
		Password:        os.Getenv("DB_PASSWORD"),
		Name:            envOr("DB_NAME", "sofascore"),
		MaxOpenConns:    1,
		MaxIdleConns:    0,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	gdb, sqlDB, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { gdbSQL, _ := gdb.DB(); gdbSQL.Close() })

	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections=%d, want 1", stats.MaxOpenConnections)
	}
}

func TestPoolWaitCountWithMaxOpenOne(t *testing.T) {
	cfg := config.Database{
		Host:            envOr("DB_HOST", "localhost"),
		Port:            envOr("DB_PORT", "3306"),
		User:            envOr("DB_USER", "root"),
		Password:        os.Getenv("DB_PASSWORD"),
		Name:            envOr("DB_NAME", "sofascore"),
		MaxOpenConns:    1,
		MaxIdleConns:    0,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	gdb, sqlDB, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { gdbSQL, _ := gdb.DB(); gdbSQL.Close() })

	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}

	held := make(chan struct{})
	release := make(chan struct{})
	secondDone := make(chan struct{})

	go func() {
		tx, err := sqlDB.Begin()
		if err != nil {
			t.Errorf("Begin 1: %v", err)
			close(held)
			return
		}
		close(held)
		<-release
		tx.Rollback()
	}()

	go func() {
		<-held
		tx, err := sqlDB.Begin()
		if err != nil {
			t.Errorf("Begin 2: %v", err)
			close(secondDone)
			return
		}
		tx.Rollback()
		close(secondDone)
	}()

	time.Sleep(200 * time.Millisecond)

	stats := sqlDB.Stats()
	if stats.WaitCount == 0 {
		t.Errorf("WaitCount=%d, expected > 0 (pool should have made the second goroutine wait)", stats.WaitCount)
	}

	close(release)
	<-secondDone

	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := sqlDB.Ping(); err == nil {
		t.Fatal("Ping should fail after Close")
	}
}
