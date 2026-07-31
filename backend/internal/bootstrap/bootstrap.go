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
}

func Init(env config.Env, version string) *App {
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
		&models.HallElement{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// ponytail: backfill name_pinyin for all guests (safe to re-run, idempotent)
	var guests []models.GuestRecord
	db.Find(&guests)
	for _, g := range guests {
		py := models.GenerateNamePinyin(g.Name)
		if py != g.NamePinyin {
			db.Model(&models.GuestRecord{}).Where("id = ?", g.ID).Update("name_pinyin", py)
		}
	}

	// ponytail: GIN indexes for trigram search (ILIKE '%..%')
	db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_name_trgm ON guest_records USING gin (name gin_trgm_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_name_pinyin_trgm ON guest_records USING gin (name_pinyin gin_trgm_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_phone_trgm ON guest_records USING gin (phone gin_trgm_ops)")

	// ponytail: FK lookup indexes
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users (email)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_banquet_tables_wedding_name ON banquet_tables (wedding_id, name)")

	if db.Migrator().HasColumn(&models.HallElement{}, "label") {
		db.Migrator().DropColumn(&models.HallElement{}, "label")
	}
	// Seed default elements for weddings that have none
	var allWeddings []uuid.UUID
	db.Model(&models.WeddingEvent{}).Pluck("id", &allWeddings)
	for _, wid := range allWeddings {
		var n int64
		db.Model(&models.HallElement{}).Where("wedding_id = ?", wid).Count(&n)
		if n == 0 {
			if err := db.Create(models.DefaultElements(wid)).Error; err != nil {
				log.Println("Warning: seed default elements for wedding", wid, ":", err)
			}
		}
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
	layoutRepo := repository.NewLayoutRepo(db)

	authService := services.NewAuthService(adminRepo, weddingRepo, tokenRepo, env.JWTSecret)
	tableService := services.NewTableService(tableRepo)
	guestService := services.NewGuestService(guestRepo, tableRepo)
	weddingService := services.NewWeddingService(weddingRepo)
	layoutService := services.NewLayoutService(layoutRepo)
	reportService := services.NewReportService(guestRepo, tableRepo, weddingRepo)

	server := config.NewFuegoServer(env, version)

	fuego.Use(server, middleware.ProxyAwareMiddleware)
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

	handlers.RegisterRoutes(server, authService, guestService, tableService, weddingService, layoutService, reportService, adminRepo)

	// Serve static frontend (SPA fallback to index.html)
	staticDir := getEnv("STATIC_DIR", "./static")
	server.Mux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue("path")
		if path == "" {
			path = "index.html"
		}
		filePath := filepath.Join(staticDir, path)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			return
		}
		// SPA fallback: serve index.html for client-side routing
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

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
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
