package database

import (
	"fmt"
	"log"
	"os"

	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Connect creates a connection to the MariaDB database using GORM.
// It reads connection settings from environment variables:
//
//	DB_HOST     (default: localhost)
//	DB_PORT     (default: 3306)
//	DB_USER     (default: root)
//	DB_PASSWORD (default: "")
//	DB_NAME     (default: sofascore)
func Connect() (*gorm.DB, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "root")
	password := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "sofascore")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, host, port, dbName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	if err := db.AutoMigrate(&models.SportEvent{}); err != nil {
		return nil, fmt.Errorf("error running auto-migration: %w", err)
	}

	log.Println("Database connection established and schema migrated.")
	return db, nil
}

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
