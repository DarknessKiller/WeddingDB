package bootstrap

import (
	"context"
	"fmt"
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

	dbURL, err := resolveDatabaseURL(env)
	if err != nil {
		log.Fatal(err)
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
		&models.AppliedMutation{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// Phone numbers are no longer collected. Drop the legacy column so old
	// databases match the model; fresh databases skip this.
	if db.Migrator().HasColumn(&models.GuestRecord{}, "phone") {
		if err := db.Migrator().DropColumn(&models.GuestRecord{}, "phone"); err != nil {
			log.Printf("warning: could not drop legacy guest_records.phone: %v", err)
		}
	}

	// Backfill name_pinyin only for rows missing it. A full scan here reads every
	// guest ever added on each boot, so it grows with total history forever.
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_guest_records_missing_pinyin ON guest_records (id) WHERE name_pinyin IS NULL OR name_pinyin = ''")
	var missing []models.GuestRecord
	warnBoot("load guests missing pinyin", db.Select("id", "name", "name_pinyin").
		Where("(name_pinyin IS NULL OR name_pinyin = '') AND name <> ''").
		Find(&missing).Error)
	for _, g := range missing {
		py := models.GenerateNamePinyin(g.Name)
		if py != g.NamePinyin {
			warnBoot("backfill pinyin for guest "+g.ID.String(), db.Model(&models.GuestRecord{}).Where("id = ?", g.ID).Update("name_pinyin", py).Error)
		}
	}

	// ponytail: GIN indexes for trigram search (ILIKE '%..%')
	execBoot(db, "CREATE EXTENSION IF NOT EXISTS pg_trgm")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_guest_records_name_trgm ON guest_records USING gin (name gin_trgm_ops)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_guest_records_name_pinyin_trgm ON guest_records USING gin (name_pinyin gin_trgm_ops)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_id ON guest_records (wedding_id, id)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_name ON guest_records (wedding_id, name)")

	// ponytail: FK lookup indexes
	execBoot(db, "CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users (email)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_banquet_tables_wedding_name ON banquet_tables (wedding_id, name)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_refresh_tokens_wedding_id ON refresh_tokens (wedding_id)")
	execBoot(db, "CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at)")

	// Expired refresh tokens are unusable; drop them so the table does not grow
	// forever. Keep a week for debugging.
	warnBoot("delete expired refresh tokens", db.Where("expires_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&models.RefreshToken{}).Error)

	if db.Migrator().HasColumn(&models.HallElement{}, "label") {
		if err := db.Migrator().DropColumn(&models.HallElement{}, "label"); err != nil {
			log.Printf("warning: could not drop legacy hall_elements.label: %v", err)
		}
	}

	redisURL := env.RedisURL
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal("Invalid REDIS_URL:", err)
	}
	rdb := redis.NewClient(redisOpts)
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
	tableService := services.NewTableService(tableRepo, guestRepo, sseHub, db)
	guestService := services.NewGuestService(guestRepo, tableRepo, sseHub, db)
	weddingService := services.NewWeddingService(weddingRepo)
	layoutService := services.NewLayoutService(layoutRepo)
	reportService := services.NewReportService(guestRepo, tableRepo, weddingRepo)

	// Global middlewares wrap the whole mux, so CORS also answers OPTIONS
	// requests no route matches. They are listed innermost first:
	// ProxyAware must run outside tracing: it rewrites RemoteAddr in place, and
	// both fuego's logger and the tracing span read RemoteAddr afterwards.
	globalMiddlewares := []func(http.Handler) http.Handler{
		middleware.CORSMiddleware,
		tracing.ServerTimingMiddleware,
	}
	if tracingEnabled {
		globalMiddlewares = append(globalMiddlewares, tracing.Middleware)
	}
	globalMiddlewares = append(globalMiddlewares, middleware.ProxyAwareMiddleware)
	server := config.NewFuegoServer(env, version, globalMiddlewares...)

	// Serve uploaded files (block directory listing)
	server.Mux.HandleFunc("GET /uploads/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		// Reject dotfiles too: the upload index (.hash-index) lives beside the
		// images and is not a public asset.
		if file == "" || strings.Contains(file, "/") || strings.Contains(file, "..") || strings.HasPrefix(file, ".") {
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
		if err := checkBootstrapPassword(password); err != nil {
			log.Fatal(err)
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

// resolveDatabaseURL prefers DATABASE_URL and otherwise builds one from explicit
// DB_* variables. Outside Compose (which injects DATABASE_URL) nothing is
// defaulted: an unset credential or sslmode must fail instead of silently
// connecting to localhost over plaintext.
func resolveDatabaseURL(env config.Env) (string, error) {
	if env.DatabaseURL != "" {
		return env.DatabaseURL, nil
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL, nil
	}
	required := []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"}
	values := make(map[string]string, len(required))
	var missing []string
	for _, key := range required {
		values[key] = os.Getenv(key)
		if values[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("DATABASE_URL is not set and %s must be set", strings.Join(missing, ", "))
	}
	return "postgresql://" + values["DB_USER"] + ":" + url.QueryEscape(values["DB_PASSWORD"]) + "@" +
		values["DB_HOST"] + ":" + values["DB_PORT"] + "/" + values["DB_NAME"] + "?sslmode=" + values["DB_SSLMODE"], nil
}

// checkBootstrapPassword applies the same policy as account creation, so a
// weak bootstrap value (the .env.example placeholder included) cannot seed the
// first admin.
func checkBootstrapPassword(password string) error {
	if password == "" {
		return fmt.Errorf("ADMIN_BOOTSTRAP_PASSWORD is required when creating the first admin")
	}
	if err := services.ValidatePassword(password); err != nil {
		return fmt.Errorf("ADMIN_BOOTSTRAP_PASSWORD %w; generate one with: openssl rand -hex 32", err)
	}
	return nil
}

// warnBoot logs a failed best-effort boot step (index, backfill, cleanup):
// the server still starts, slower or with stale data, rather than not at all.
func warnBoot(step string, err error) {
	if err != nil {
		log.Printf("warning: boot step %s: %v", step, err)
	}
}

// execBoot runs a best-effort boot statement and logs a failure.
func execBoot(db *gorm.DB, stmt string) {
	warnBoot(stmt, db.Exec(stmt).Error)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
