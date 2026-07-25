package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Env struct {
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	Port        string
}

func LoadEnv() Env {
	godotenv.Load()
	return Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        getEnv("PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
