package config

import "os"

type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type Config struct {
	APIAddr          string
	JWTSecret        string
	APKStoragePath   string
	ImageStoragePath string
	Database         Database
}

func Load() (Config, error) {
	return Config{
		APIAddr:          getEnv("API_ADDR", ":8080"),
		JWTSecret:        getEnv("JWT_SECRET", "changeme-please-set-JWT_SECRET-env"),
		APKStoragePath:   getEnv("APK_STORAGE_PATH", "./apk_storage"),
		ImageStoragePath: getEnv("IMAGE_STORAGE_PATH", "./image_storage"),
		Database: Database{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "sofascore"),
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
