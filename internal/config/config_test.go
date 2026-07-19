package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	if !errors.Is(err, ErrJWTSecretRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadSecurityDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Redis.URL != "redis://localhost:6379/0" {
		t.Fatalf("Redis.URL=%q", cfg.Redis.URL)
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Fatal(cfg.HTTP.ReadHeaderTimeout)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("API_ADDR", "")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.APIAddr != ":8080" {
		t.Fatalf("APIAddr=%q", got.APIAddr)
	}
	if got.Database.Host != "localhost" {
		t.Fatalf("Host=%q", got.Database.Host)
	}
}

func TestLoadAllDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.JWTSecret != "test-secret" {
		t.Fatalf("JWTSecret=%q", cfg.JWTSecret)
	}
	if cfg.APKStoragePath != "./apk_storage" {
		t.Fatalf("APKStoragePath=%q", cfg.APKStoragePath)
	}
	if cfg.ImageStoragePath != "./image_storage" {
		t.Fatalf("ImageStoragePath=%q", cfg.ImageStoragePath)
	}
	if cfg.Database.Port != "3306" {
		t.Fatalf("Port=%q", cfg.Database.Port)
	}
	if cfg.Database.User != "root" {
		t.Fatalf("User=%q", cfg.Database.User)
	}
	if cfg.Database.Name != "sofascore" {
		t.Fatalf("Name=%q", cfg.Database.Name)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("DB_PASSWORD", "pass123")
	t.Setenv("API_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.JWTSecret != "super-secret" {
		t.Fatalf("JWTSecret=%q", cfg.JWTSecret)
	}
	if cfg.Database.Password != "pass123" {
		t.Fatalf("Password=%q", cfg.Database.Password)
	}
	if cfg.APIAddr != ":9090" {
		t.Fatalf("APIAddr=%q", cfg.APIAddr)
	}
}

func TestLoadRedisDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Redis.DialTimeout != 5*time.Second {
		t.Fatalf("DialTimeout=%v", cfg.Redis.DialTimeout)
	}
	if cfg.Redis.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout=%v", cfg.Redis.ReadTimeout)
	}
	if cfg.Redis.WriteTimeout != 3*time.Second {
		t.Fatalf("WriteTimeout=%v", cfg.Redis.WriteTimeout)
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
}

func TestDatabasePoolDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Fatalf("MaxOpenConns=%d, want 25", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Fatalf("MaxIdleConns=%d, want 10", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("ConnMaxLifetime=%v, want 30m", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Database.ConnMaxIdleTime != 5*time.Minute {
		t.Fatalf("ConnMaxIdleTime=%v, want 5m", cfg.Database.ConnMaxIdleTime)
	}
}

func TestDatabasePoolEnvOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "50")
	t.Setenv("DB_MAX_IDLE_CONNS", "20")
	t.Setenv("DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "2m")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.MaxOpenConns != 50 {
		t.Fatalf("MaxOpenConns=%d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 20 {
		t.Fatalf("MaxIdleConns=%d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 10*time.Minute {
		t.Fatalf("ConnMaxLifetime=%v", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Database.ConnMaxIdleTime != 2*time.Minute {
		t.Fatalf("ConnMaxIdleTime=%v", cfg.Database.ConnMaxIdleTime)
	}
}

func TestValidatePoolRejectsNonpositiveMaxOpen(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    0,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for MaxOpenConns=0")
	}
}

func TestValidatePoolRejectsNegativeMaxIdle(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    25,
			MaxIdleConns:    -1,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for MaxIdleConns=-1")
	}
}

func TestValidatePoolRejectsMaxIdleGreaterThanMaxOpen(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    10,
			MaxIdleConns:    20,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for MaxIdleConns > MaxOpenConns")
	}
}

func TestValidatePoolRejectsNonpositiveLifetime(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for ConnMaxLifetime=0")
	}
}

func TestValidatePoolRejectsNonpositiveIdleTime(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 0,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for ConnMaxIdleTime=0")
	}
}

func TestValidatePoolAcceptsValidConfig(t *testing.T) {
	cfg := Config{
		JWTSecret: "test",
		Database: Database{
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
