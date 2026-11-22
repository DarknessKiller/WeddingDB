package main

import (
	"context"
	"log"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Migrate runs the schema and data migrations once. Run it before the server
// on every deploy; the server no longer applies DDL at boot.
func main() {
	env := config.LoadEnv()
	db, err := gorm.Open(postgres.Open(bootstrap.DatabaseURL(env)), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	if err := bootstrap.Migrate(context.Background(), db); err != nil {
		log.Fatal("Migration failed:", err)
	}
}
