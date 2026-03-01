package config

import (
	"os"
)

type Config struct {
	Address      string
	JWTSecret    string
	DefaultAdmin DefaultAdmin
}

type DefaultAdmin struct {
	FullName string
	Email    string
	Password string
}

func Load() Config {
	return Config{
		Address:   getenv("APP_ADDRESS", ":8080"),
		JWTSecret: getenv("APP_JWT_SECRET", "development-secret"),
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
