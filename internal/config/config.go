package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var ErrJWTSecretRequired = errors.New("JWT_SECRET is required")

type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type Redis struct {
	URL          string
	KeyPrefix    string
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type HTTP struct {
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type Config struct {
	APIAddr          string
	JWTSecret        string
	APKStoragePath   string
	ImageStoragePath string
	Database         Database
	Redis            Redis
	HTTP             HTTP
}

func Load() (Config, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return Config{}, ErrJWTSecretRequired
	}

	return Config{
		APIAddr:          getEnv("API_ADDR", ":8080"),
		JWTSecret:        secret,
		APKStoragePath:   getEnv("APK_STORAGE_PATH", "./apk_storage"),
		ImageStoragePath: getEnv("IMAGE_STORAGE_PATH", "./image_storage"),
		Database: Database{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "sofascore"),
		},
		Redis: Redis{
			URL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
			KeyPrefix:    getEnv("REDIS_KEY_PREFIX", ""),
			DialTimeout:  getDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getDuration("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
		},
		HTTP: HTTP{
			ReadHeaderTimeout: getDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			WriteTimeout:      getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:       getDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: invalid duration for %s=%q, using default %s: %v\n", key, v, fallback, err)
		return fallback
	}
	return d
}
