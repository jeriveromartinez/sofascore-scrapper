package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrJWTSecretRequired = errors.New("JWT_SECRET is required")
	ErrJWTSecretTooShort  = errors.New("JWT_SECRET is too short")
)

// MinJWTSecretLength is the minimum acceptable length for JWT_SECRET.
// 32 characters is the project policy (see docs/operations/runbook.md).
const MinJWTSecretLength = 32

type Database struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
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
	TrustedProxies    []string
}

type Config struct {
	APIAddr           string
	PprofAddr         string
	PprofEnabled      bool
	JWTSecret         string
	APKStoragePath    string
	ImageStoragePath  string
	Database          Database
	Redis             Redis
	HTTP              HTTP
	ScrapeBatchSize   int
	ScrapeConcurrency int
	// PushTimerTickInterval is how often the timers runner wakes up
	// to dispatch due scheduled pushes. Default 5s balances latency
	// against DB load; smaller intervals cost more queries per
	// minute, larger intervals stretch the perceived delay between
	// the cron tick and the user-visible push.
	PushTimerTickInterval time.Duration
	// PushTimerBatchLimit caps the number of timers the runner
	// processes per tick. Default 100 keeps a single tick
	// bounded; raise it when the campaign mix is dominated by
	// large audiences.
	PushTimerBatchLimit int
	// SkipMigrate, when true, skips database AutoMigrate and the
	// default-admin seeder on startup. Used when the operator manages
	// schema externally or wants a hot path with no DB write.
	SkipMigrate bool
}

func Load() (Config, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return Config{}, ErrJWTSecretRequired
	}
	if len(secret) < MinJWTSecretLength {
		return Config{}, fmt.Errorf("%w: must be at least %d characters (got %d)", ErrJWTSecretTooShort, MinJWTSecretLength, len(secret))
	}

	return Config{
		APIAddr:           getEnv("API_ADDR", ":8080"),
		PprofAddr:         getEnv("PPROF_ADDR", ""),
		PprofEnabled:      getBool("ENABLE_PPROF", false),
		JWTSecret:         secret,
		APKStoragePath:    getEnv("APK_STORAGE_PATH", "./apk_storage"),
		ImageStoragePath:  getEnv("IMAGE_STORAGE_PATH", "./image_storage"),
		ScrapeBatchSize:        getInt("SCRAPE_BATCH_SIZE", 500),
		ScrapeConcurrency:      getInt("SCRAPE_CONCURRENCY", 8),
		PushTimerTickInterval:  getDuration("PUSH_TIMER_TICK_INTERVAL", 5*time.Second),
		PushTimerBatchLimit:    getInt("PUSH_TIMER_BATCH_LIMIT", 100),
		SkipMigrate:            getBool("SKIP_MIGRATE", false),
		Database: Database{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "3306"),
			User:            getEnv("DB_USER", "root"),
			Password:        getEnv("DB_PASSWORD", ""),
			Name:            getEnv("DB_NAME", "sofascore"),
			MaxOpenConns:    getInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
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
			TrustedProxies:    getCSV("TRUSTED_PROXIES"),
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getCSV(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
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

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: invalid integer for %s=%q, using default %d: %v\n", key, v, fallback, err)
		return fallback
	}
	return i
}

// getBool reads a boolean env var. Accepts the same set as
// strconv.ParseBool ("1", "t", "T", "TRUE", "true", "True", "0",
// "f", "F", "FALSE", "false", "False"). Empty string returns the
// fallback; an invalid value falls back to fallback and writes a
// warning to stderr.
func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: invalid boolean for %s=%q, using default %v: %v\n", key, v, fallback, err)
		return fallback
	}
	return b
}

func (c Config) Validate() error {
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("config: MaxOpenConns must be positive, got %d", c.Database.MaxOpenConns)
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("config: MaxIdleConns must be non-negative, got %d", c.Database.MaxIdleConns)
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("config: MaxIdleConns (%d) must not exceed MaxOpenConns (%d)", c.Database.MaxIdleConns, c.Database.MaxOpenConns)
	}
	if c.Database.ConnMaxLifetime <= 0 {
		return fmt.Errorf("config: ConnMaxLifetime must be positive, got %v", c.Database.ConnMaxLifetime)
	}
	if c.Database.ConnMaxIdleTime <= 0 {
		return fmt.Errorf("config: ConnMaxIdleTime must be positive, got %v", c.Database.ConnMaxIdleTime)
	}
	return nil
}
