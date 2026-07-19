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
