package bootstrap

import (
	"context"
	"log"
	"net/url"
	"os"
	"strings"
	"weddingdb/internal/config"
	"weddingdb/internal/handlers"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type App struct {
	Server      *fuego.Server
	DB          *gorm.DB
	Redis       *redis.Client
	AuthService *services.AuthService
	NonceStore  *middleware.NonceStore
}

func Init(env config.Env) *App {
	// Parse DATABASE_URL or use individual params
	dbURL := env.DatabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		// Build from individual env vars (for NocoDB compatibility)
		host := getEnv("DB_HOST", "localhost")
		port := getEnv("DB_PORT", "5432")
		user := getEnv("DB_USER", "postgres")
		pass := getEnv("DB_PASSWORD", "")
		name := getEnv("DB_NAME", "weddingdb")
		sslmode := getEnv("DB_SSLMODE", "disable")
		dbURL = "postgresql://" + user + ":" + url.QueryEscape(pass) + "@" + host + ":" + port + "/" + name + "?sslmode=" + sslmode
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	if err := db.AutoMigrate(
		&models.WeddingEvent{},
		&models.AdminUser{},
		&models.BanquetTable{},
		&models.GuestRecord{},
		&models.RefreshToken{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// Redis
	redisAddr := env.RedisURL
	if redisAddr == "" {
		redisAddr = "redis://localhost:6379"
	}
	// Strip redis:// scheme for go-redis
	redisAddr = strings.TrimPrefix(redisAddr, "redis://")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	// Repos
	adminRepo := repository.NewAdminRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	tableRepo := repository.NewTableRepo(db)
	guestRepo := repository.NewGuestRepo(db)
	tokenRepo := repository.NewTokenRepo(db)

	// Services
	authService := services.NewAuthService(adminRepo, tokenRepo, env.JWTSecret)
	nonceStore := middleware.NewNonceStore(rdb)
	tableService := services.NewTableService(tableRepo)
	guestService := services.NewGuestService(guestRepo, tableRepo)
	weddingService := services.NewWeddingService(weddingRepo)

	server := config.NewFuegoServer(env)

	// CORS — must be first
	fuego.Use(server, middleware.CORSMiddleware)

	// Register routes
	handlers.RegisterRoutes(server, authService, guestService, tableService, weddingService, adminRepo, nonceStore)

	// Seed default admin if none exists
	var count int64
	db.Model(&models.AdminUser{}).Count(&count)
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		admin := &models.AdminUser{
			Email:    "admin@weddingdb.local",
			Password: string(hash),
			Name:     "Admin",
			Role:     "service_admin",
		}
		if err := adminRepo.Create(admin); err != nil {
			log.Println("Warning: failed to seed admin:", err)
		} else {
			log.Println("Seeded default admin: admin@weddingdb.local / admin123")
		}
	}

	return &App{
		Server:      server,
		DB:          db,
		Redis:       rdb,
		AuthService: authService,
		NonceStore:  nonceStore,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
