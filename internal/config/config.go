package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
)

var (
	ErrJWTSecretRequired = errors.New("JWT_SECRET is required")
	ErrJWTSecretTooShort = errors.New("JWT_SECRET is too short")
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
	// FotMobTimezone is appended to the
	// /api/data/matches URL as `?timezone=…`; FotMob uses it to
	// bucket matches into "today/yesterday/tomorrow". Defaults to
	// Europe/Paris so the daily scrape lines up with FotMob's
	// editorial day (matches are published with the venue's local
	// evening as "today's slate").
	FotMobTimezone string
	// Scores365 configures the 365scores web API client, the source
	// for every sport FotMob does not cover.
	Scores365 Scores365
}

// Scores365 holds the query parameters webws.365scores.com expects on
// every request. They are exposed as environment variables because a
// deployment may want a different locale or day boundary than the
// es-mx defaults the site itself serves.
type Scores365 struct {
	// LangID selects the language of the localized strings in the
	// payload (competition names, competitor names, status labels).
	// 29 is es-mx, matching https://www.365scores.com/es-mx.
	//
	// Note: the fixture status classifier keys off the localized
	// status text, and it only carries keyword tables for Spanish
	// and English (see internal/scraper/scores365/feed.go). Pointing
	// this at a third language degrades cancelled/postponed fixtures
	// to "finished".
	LangID int
	// Timezone is the IANA zone the API uses to bucket fixtures into
	// calendar days and to stamp each game's startTime offset.
	// Defaults to UTC so the scrape loop's day boundary matches the
	// UTC dates it iterates.
	Timezone string
	// CountryID scopes the popularity ordering of the payload. It
	// does not filter the result set. 116 is what the es-mx site
	// sends.
	CountryID int
	// DiscoverySports overrides the sport-slug list the boot-time
	// catalog discovery sweep enrols. Empty means "use the catalog
	// package's default list", which excludes football because
	// FotMob remains the primary football source.
	DiscoverySports []string
}

func Load() (Config, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return Config{}, ErrJWTSecretRequired
	}
	if len(secret) < MinJWTSecretLength {
		return Config{}, fmt.Errorf("%w: must be at least %d characters (got %d)", ErrJWTSecretTooShort, MinJWTSecretLength, len(secret))
	}

	cfg := Config{
		APIAddr:               getEnv("API_ADDR", ":8080"),
		PprofAddr:             getEnv("PPROF_ADDR", ""),
		PprofEnabled:          getBool("ENABLE_PPROF", false),
		JWTSecret:             secret,
		APKStoragePath:        getEnv("APK_STORAGE_PATH", "./apk_storage"),
		ImageStoragePath:      getEnv("IMAGE_STORAGE_PATH", "./image_storage"),
		ScrapeBatchSize:       getInt("SCRAPE_BATCH_SIZE", 500),
		ScrapeConcurrency:     getInt("SCRAPE_CONCURRENCY", 8),
		PushTimerTickInterval: getDuration("PUSH_TIMER_TICK_INTERVAL", 5*time.Second),
		PushTimerBatchLimit:   getInt("PUSH_TIMER_BATCH_LIMIT", 100),
		SkipMigrate:           getBool("SKIP_MIGRATE", false),
		FotMobTimezone:        getEnv("FOTMOB_TIMEZONE", "Europe/Paris"),
		Scores365: Scores365{
			LangID:          getInt("SCORES365_LANG_ID", scores365.DefaultLangID),
			Timezone:        getEnv("SCORES365_TIMEZONE", scores365.DefaultTimezoneName),
			CountryID:       getInt("SCORES365_COUNTRY_ID", scores365.DefaultUserCountryID),
			DiscoverySports: getCSV("SCORES365_DISCOVERY_SPORTS"),
		},
		Database: Database{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "3306"),
			User:            getEnv("DB_USER", "root"),
			Password:        getEnv("DB_PASSWORD", ""),
			Name:            getEnv("DB_NAME", "scores365"),
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
		// The write deadline starts when the request header is read,
		// so it must cover the whole request: the APK endpoints stream
		// large files as the response body and accept 10 MiB chunks,
		// both slower than the 15m read timeout the server allows.
		HTTP: HTTP{
			ReadHeaderTimeout: getDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			WriteTimeout:      getDuration("HTTP_WRITE_TIMEOUT", 15*time.Minute),
			IdleTimeout:       getDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
			TrustedProxies:    getCSV("TRUSTED_PROXIES"),
		},
	}
	// Validate here so an unusable pool configuration fails the boot
	// instead of surfacing later as a runtime connection error.
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
