package database

import (
	"database/sql"
	"fmt"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(cfg config.Database) (*gorm.DB, *sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
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
