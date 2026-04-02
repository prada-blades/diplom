package config

import (
	"os"
	"strconv"
)

type Config struct {
	Address      string
	DatabaseURL  string
	JWTSecret    string
	Redis        RedisConfig
	DefaultAdmin DefaultAdmin
}

type RedisConfig struct {
	Enabled  bool
	Addr     string
	Password string
	DB       int
}

type DefaultAdmin struct {
	FullName string
	Email    string
	Password string
}

func Load() Config {
	return Config{
		Address:     getenv("APP_ADDRESS", ":8080"),
		DatabaseURL: getenv("APP_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/diplom?sslmode=disable"),
		JWTSecret:   getenv("APP_JWT_SECRET", "development-secret"),
		Redis: RedisConfig{
			Enabled:  getenv("APP_REDIS_ENABLED", "false") == "true",
			Addr:     getenv("APP_REDIS_ADDR", "localhost:6379"),
			Password: getenv("APP_REDIS_PASSWORD", ""),
			DB:       getenvInt("APP_REDIS_DB", 0),
		},
		DefaultAdmin: DefaultAdmin{
			FullName: getenv("APP_ADMIN_NAME", "System Administrator"),
			Email:    getenv("APP_ADMIN_EMAIL", "admin@corp.local"),
			Password: getenv("APP_ADMIN_PASSWORD", "admin123"),
		},
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
