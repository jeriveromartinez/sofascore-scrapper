package database

import (
	"fmt"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(cfg config.Database) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return db, nil
}
