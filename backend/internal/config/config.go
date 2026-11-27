package config

import (
	"cmp"
	"log"
	"os"
	"unicode/utf8"

	"github.com/joho/godotenv"
)

type Env struct {
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	Port        string
	PublicURL   string
}

func LoadEnv() Env {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env: %v", err)
	}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is required and must not be empty")
	}
	if secret == "change-me-in-production" || secret == "change-me-to-a-random-secret" {
		log.Fatal("JWT_SECRET must not be a value shipped in .env.example; generate one with: openssl rand -hex 32")
	}
	if utf8.RuneCountInString(secret) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 characters")
	}
	port := cmp.Or(os.Getenv("PORT"), "8080")
	publicURL := cmp.Or(os.Getenv("PUBLIC_URL"), "http://localhost:"+port)
	return Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    cmp.Or(os.Getenv("REDIS_URL"), "redis://localhost:6379"),
		JWTSecret:   secret,
		Port:        port,
		PublicURL:   publicURL,
	}
}
