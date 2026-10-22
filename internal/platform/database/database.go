package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// slowQueryThreshold is how long a statement may run before GORM reports
// it as slow.
const slowQueryThreshold = 200 * time.Millisecond

func Open(cfg config.Database) (*gorm.DB, *sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newGormLogger(slog.Default()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	return db, sqlDB, nil
}

// newGormLogger builds the logger GORM reports through.
//
// The stock logger writes multi-line blocks to stdout for every error,
// including the misses the application handles on purpose: the boot
// discovery sweep alone produced about 1900 of them. It also renders
// the trace SQL with the bound values interpolated, which put device
// tokens, refresh tokens and password hashes into the log.
func newGormLogger(log *slog.Logger) logger.Interface {
	if log == nil {
		log = slog.Default()
	}
	return logger.NewSlogLogger(log, logger.Config{
		LogLevel:                  logger.Warn,
		SlowThreshold:             slowQueryThreshold,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
	})
}
