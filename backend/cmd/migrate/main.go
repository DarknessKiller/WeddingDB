package main

import (
	"context"
	"log"
	"os"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Migrate runs the schema and data migrations once. Run it before the server
// on every deploy; the server no longer applies DDL at boot.
// Only the database settings are read: the job needs no JWT secret or Redis.
func main() {
	// Match the server's env loading without config.LoadEnv, which requires JWT_SECRET.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env: %v", err)
	}
	db, err := gorm.Open(postgres.Open(bootstrap.DatabaseURL(config.Env{})), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	if err := bootstrap.Migrate(context.Background(), db); err != nil {
		log.Fatal("Migration failed:", err)
	}
}
