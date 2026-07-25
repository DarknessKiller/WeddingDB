package config

import (
	"log"
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
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is required and must not be empty")
	}
	return Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   secret,
		Port:        getEnv("PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
