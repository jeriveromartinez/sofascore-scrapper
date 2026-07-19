package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("API_ADDR", "")
	t.Setenv("DB_HOST", "localhost")
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
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.JWTSecret != "changeme-please-set-JWT_SECRET-env" {
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
