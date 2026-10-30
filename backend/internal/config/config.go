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
	if secret == "change-me-in-production" || secret == "change-me-to-a-random-secret" {
		log.Fatal("JWT_SECRET must not be a value shipped in .env.example; generate one with: openssl rand -hex 32")
	}
	port := getEnv("PORT", "8080")
	return Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   secret,
		Port:        port,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
