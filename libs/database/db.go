package database

import (
	"log"
	"os"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	newdb "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	"gorm.io/gorm"
)

var _db *gorm.DB

func Connect() (*gorm.DB, error) {
	if _db != nil {
		return _db, nil
	}

	cfg := config.Database{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "root"),
		Password: getEnv("DB_PASSWORD", ""),
		Name:     getEnv("DB_NAME", "sofascore"),
	}

	db, err := newdb.Open(cfg)
	if err != nil {
		return nil, err
	}

	log.Println("Database connection established and schema migrated.")
	_db = db
	return _db, nil
}

func GetDB() (*gorm.DB, error) {
	if _db == nil {
		return Connect()
	}

	return _db, nil
}

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
