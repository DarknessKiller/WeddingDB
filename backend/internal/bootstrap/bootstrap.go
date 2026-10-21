package bootstrap

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"weddingdb/internal/config"
	"weddingdb/internal/handlers"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/tracing"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type App struct {
	Server          *fuego.Server
	DB              *gorm.DB
	Redis           *redis.Client
	AuthService     *services.AuthService
	SSEHub          *services.SSEHub
	ShutdownTracing func(context.Context) error
}

func Init(env config.Env, version string) *App {
	traceShutdown, tracingEnabled := tracing.Setup(version)

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
	if sqlDB, err := db.DB(); err == nil {
		// Bound the pool so a wedding-night burst cannot exhaust Postgres.
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(20)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	if tracingEnabled {
		if err := db.Use(tracing.NewGormPlugin()); err != nil {
			log.Println("Warning: failed to register tracing GORM plugin:", err)
		}
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

	// Backfill name_pinyin only for rows missing it. A full scan here reads every
	// guest ever added on each boot, so it grows with total history forever.
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_missing_pinyin ON guest_records (id) WHERE name_pinyin IS NULL OR name_pinyin = ''")
	var missing []models.GuestRecord
	db.Select("id", "name", "name_pinyin").
		Where("(name_pinyin IS NULL OR name_pinyin = '') AND name <> ''").
		Find(&missing)
	for _, g := range missing {
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
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_id ON guest_records (wedding_id, id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_name ON guest_records (wedding_id, name)")

	// ponytail: FK lookup indexes
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users (email)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_banquet_tables_wedding_name ON banquet_tables (wedding_id, name)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_refresh_tokens_wedding_id ON refresh_tokens (wedding_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at)")

	// Expired refresh tokens are unusable; drop them so the table does not grow
	// forever. Keep a week for debugging.
	db.Where("expires_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&models.RefreshToken{})

	if db.Migrator().HasColumn(&models.HallElement{}, "label") {
		db.Migrator().DropColumn(&models.HallElement{}, "label")
	}
	// Seed default elements for weddings that have none — one query, not one
	// COUNT per wedding on every boot.
	var missingHall []uuid.UUID
	db.Model(&models.WeddingEvent{}).
		Where("NOT EXISTS (SELECT 1 FROM hall_elements WHERE hall_elements.wedding_id = wedding_events.id)").
		Pluck("id", &missingHall)
	for _, wid := range missingHall {
		if err := db.Create(models.DefaultElements(wid)).Error; err != nil {
			log.Println("Warning: seed default elements for wedding", wid, ":", err)
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

	sseHub := services.NewSSEHub(rdb)

	authService := services.NewAuthService(adminRepo, weddingRepo, tokenRepo, env.JWTSecret, rdb)
	tableService := services.NewTableService(tableRepo, guestRepo, sseHub)
	guestService := services.NewGuestService(guestRepo, tableRepo, sseHub, db)
	weddingService := services.NewWeddingService(weddingRepo)
	layoutService := services.NewLayoutService(layoutRepo)
	reportService := services.NewReportService(guestRepo, tableRepo, weddingRepo)

	server := config.NewFuegoServer(env, version)

	// ProxyAware must run outside tracing: it rewrites RemoteAddr in place, and
	// both fuego's logger and the tracing span read RemoteAddr afterwards.
	// Fuego executes middlewares outer-first, so proxy is registered first.
	fuego.Use(server, middleware.ProxyAwareMiddleware)
	fuego.Use(server, tracing.ServerTimingMiddleware)
	if tracingEnabled {
		fuego.Use(server, tracing.Middleware)
	}
	fuego.Use(server, middleware.CORSMiddleware)

	// Serve uploaded files (block directory listing)
	server.Mux.HandleFunc("GET /uploads/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		if file == "" || strings.Contains(file, "/") || strings.Contains(file, "..") {
			http.NotFound(w, r)
			return
		}
		filePath := filepath.Join("./uploads", file)
		// keys are forgiving per-API: admins may replace uploaded files (e.g. kiosk logo/background)
		w.Header().Set("Cache-Control", "public, max-age=604800, stale-while-revalidate=86400")
		// Uploads can contain user-supplied SVG: sandbox it so a navigated-to
		// upload cannot run script in the app's origin (token theft via
		// localStorage). <img> embedding still renders — images never run scripts.
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, filePath)
	})

	handlers.RegisterRoutes(server, authService, guestService, tableService, weddingService, layoutService, reportService, adminRepo)

	// SSE endpoint for real-time guest event streaming (raw handler, bypasses Fuego).
	sseHandler := handlers.NewSSEHandler(sseHub, authService)
	server.Mux.HandleFunc("GET /api/weddings/{wid}/events", sseHandler.StreamHandler())

	// Serve static frontend (SPA fallback to index.html)
	staticDir := getEnv("STATIC_DIR", "./static")
	server.Mux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) {
		// Raw mux route: Fuego middleware does not run here, so set the
		// baseline headers by hand (uploads have their own, stronger set).
		middleware.SetSecurityHeaders(w)
		path := r.PathValue("path")
		if path == "" {
			path = "index.html"
		}
		filePath := filepath.Join(staticDir, path)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			stripped := strings.TrimPrefix(path, "/")
			if strings.HasPrefix(stripped, "_app/") || strings.HasPrefix(stripped, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if stripped == "index.html" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			if path == "favicon.svg" {
				w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=604800, stale-while-revalidate=86400")
			}
			http.ServeFile(w, r, filePath)
			return
		}
		// SPA fallback: serve index.html for client-side routing
		w.Header().Set("Cache-Control", "no-cache") // deep links land here with a non-file path: mark no-cache so browser heuristics never serve stale chunk URLs after a redeploy
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	// Seed default admin if none exists
	var count int64
	db.Model(&models.AdminUser{}).Count(&count)
	if count == 0 {
		password := os.Getenv("ADMIN_BOOTSTRAP_PASSWORD")
		if password == "" {
			log.Fatal("ADMIN_BOOTSTRAP_PASSWORD is required when creating the first admin")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
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
		if err := adminRepo.Create(context.Background(), admin); err != nil {
			log.Println("Warning: failed to seed admin:", err)
		} else {
			log.Println("Seeded default admin: admin@weddingdb.local")
		}
	}

	return &App{
		Server:          server,
		DB:              db,
		Redis:           rdb,
		AuthService:     authService,
		SSEHub:          sseHub,
		ShutdownTracing: traceShutdown,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
