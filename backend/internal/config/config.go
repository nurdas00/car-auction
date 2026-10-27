package config

import (
	"os"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	RedisAddr     string
	JWTSecret     string
	JWTTTL        time.Duration
}

func Load() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://autoauction:autoauction@localhost:5432/autoauction?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTTTL:      24 * time.Hour,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
