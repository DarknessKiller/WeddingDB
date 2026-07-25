package bootstrap

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"weddingdb/internal/config"
	"weddingdb/internal/handlers"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
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
	dbURL := env.DatabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
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
		&models.UserWedding{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	redisAddr := env.RedisURL
	if redisAddr == "" {
		redisAddr = "redis://localhost:6379"
	}
	redisAddr = strings.TrimPrefix(redisAddr, "redis://")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	adminRepo := repository.NewAdminRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	tableRepo := repository.NewTableRepo(db)
	guestRepo := repository.NewGuestRepo(db)
	tokenRepo := repository.NewTokenRepo(db)

	authService := services.NewAuthService(adminRepo, weddingRepo, tokenRepo, env.JWTSecret)
	nonceStore := middleware.NewNonceStore(rdb)
	tableService := services.NewTableService(tableRepo)
	guestService := services.NewGuestService(guestRepo, tableRepo)
	weddingService := services.NewWeddingService(weddingRepo)

	server := config.NewFuegoServer(env)

	fuego.Use(server, middleware.CORSMiddleware)

	// Serve uploaded files (block directory listing)
	server.Mux.HandleFunc("GET /uploads/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		if file == "" || strings.Contains(file, "/") || strings.Contains(file, "..") {
			http.NotFound(w, r)
			return
		}
		filePath := filepath.Join("./uploads", file)
		http.ServeFile(w, r, filePath)
	})

	handlers.RegisterRoutes(server, authService, guestService, tableService, weddingService, adminRepo, nonceStore)

	// Seed default admin if none exists
	var count int64
	db.Model(&models.AdminUser{}).Count(&count)
	if count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal("Failed to hash seed admin password:", err)
		}
		admin := &models.AdminUser{
			ID:                  uuid.New(),
			Email:               "admin@weddingdb.local",
			Password:            string(hash),
			Name:                "Admin",
			Role:                "admin",
			ForcePasswordChange: true,
		}
		if err := adminRepo.Create(admin); err != nil {
			log.Println("Warning: failed to seed admin:", err)
		} else {
			log.Println("Seeded default admin: admin@weddingdb.local")
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
