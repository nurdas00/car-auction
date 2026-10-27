package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string
	RedisAddr   string
	JWTSecret   string
	JWTTTL      time.Duration
	CORSOrigins []string
	UploadsDir  string // локальный диск для фото/видео; см. ТЗ раздел 7 про переезд на S3/R2
}

func Load() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://autoauction:autoauction@localhost:5432/autoauction?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTTTL:      24 * time.Hour,
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
		UploadsDir:  getEnv("UPLOADS_DIR", "/data/uploads"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
