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

// DatabaseURL resolves the Postgres DSN from env, falling back to the DB_* vars.
func DatabaseURL(env config.Env) string {
	if env.DatabaseURL != "" {
		return env.DatabaseURL
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL
	}
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	pass := getEnv("DB_PASSWORD", "")
	name := getEnv("DB_NAME", "weddingdb")
	sslmode := getEnv("DB_SSLMODE", "disable")
	return "postgresql://" + user + ":" + url.QueryEscape(pass) + "@" + host + ":" + port + "/" + name + "?sslmode=" + sslmode
}

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

	db, err := gorm.Open(postgres.Open(DatabaseURL(env)), &gorm.Config{})
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
	tableService := services.NewTableService(tableRepo, sseHub, db)
	guestService := services.NewGuestService(guestRepo, sseHub, db)
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
	server.Mux.HandleFunc("GET /{path...}", spaHandler(staticDir))

	// Seed default admin if none exists
	var count int64
	db.Model(&models.AdminUser{}).Count(&count)
	if count == 0 {
		password := os.Getenv("ADMIN_BOOTSTRAP_PASSWORD")
		if password == "" {
			log.Fatal("ADMIN_BOOTSTRAP_PASSWORD is required when creating the first admin")
		}
		if password == "replace-with-a-strong-one-time-password" {
			log.Fatal("ADMIN_BOOTSTRAP_PASSWORD must not be the value shipped in .env.example; generate one with: openssl rand -hex 32")
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

const (
	// migrateAdvisoryLockID is arbitrary but stable across deployments.
	migrateAdvisoryLockID = 8421337
	// pinyinBackfillPageSize bounds rows held in memory and updated per batch.
	pinyinBackfillPageSize = 500
)

// Migrate applies schema changes and one-off data backfills. Run it as a
// separate job before the server starts, not on every boot.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if db.Name() == "postgres" {
		// Serialize concurrent migrate jobs (rolling deploys, overlapping
		// one-shot service instances) on one advisory lock. The lock lives on a
		// dedicated connection so acquire and release use the same session.
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("migrate: get database handle: %w", err)
		}
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return fmt.Errorf("migrate: reserve lock connection: %w", err)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				log.Printf("warning: close migrate lock connection: %v", err)
			}
		}()
		for {
			var acquired bool
			if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrateAdvisoryLockID).Scan(&acquired); err != nil {
				// The server may have granted the lock before the response was
				// lost; release it best-effort so a pooled session cannot hold it.
				if _, uerr := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrateAdvisoryLockID); uerr != nil {
					log.Printf("warning: release migrate advisory lock after acquire error: %v", uerr)
				}
				return fmt.Errorf("migrate: acquire advisory lock: %w", err)
			}
			if acquired {
				// Register the release before anything else can return, so a
				// cancelled request cannot leave the lock on the pooled session.
				defer func() {
					if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrateAdvisoryLockID); err != nil {
						log.Printf("warning: release migrate advisory lock: %v", err)
					}
				}()
				break
			}
			log.Printf("migrate: waiting for another migrate job to release advisory lock %d", migrateAdvisoryLockID)
			select {
			case <-ctx.Done():
				return fmt.Errorf("migrate: waiting for advisory lock: %w", ctx.Err())
			case <-time.After(time.Second):
			}
		}
	}
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(
		&models.WeddingEvent{},
		&models.AdminUser{},
		&models.BanquetTable{},
		&models.GuestRecord{},
		&models.RefreshToken{},
		&models.UserWedding{},
		&models.HallElement{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	// Phone numbers are no longer collected. Drop the legacy column so old
	// databases match the model; fresh databases skip this.
	if db.Migrator().HasColumn(&models.GuestRecord{}, "phone") {
		if err := db.Migrator().DropColumn(&models.GuestRecord{}, "phone"); err != nil {
			return fmt.Errorf("drop legacy guest_records.phone: %w", err)
		}
	}

	// Backfill name_pinyin only for rows missing it.
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_guest_records_missing_pinyin ON guest_records (id) WHERE name_pinyin IS NULL OR name_pinyin = ''").Error; err != nil {
		return fmt.Errorf("create missing-pinyin index: %w", err)
	}
	var lastID uuid.UUID
	for {
		var missing []models.GuestRecord
		q := db.Select("id", "name", "name_pinyin").
			Where("(name_pinyin IS NULL OR name_pinyin = '') AND name <> ''").
			Order("id").Limit(pinyinBackfillPageSize)
		if lastID != uuid.Nil {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&missing).Error; err != nil {
			return fmt.Errorf("load guests missing pinyin: %w", err)
		}
		for _, g := range missing {
			py := models.GenerateNamePinyin(g.Name)
			if py != g.NamePinyin {
				if err := db.Model(&models.GuestRecord{}).Where("id = ?", g.ID).Update("name_pinyin", py).Error; err != nil {
					return fmt.Errorf("backfill pinyin for guest %s: %w", g.ID, err)
				}
			}
		}
		if len(missing) < pinyinBackfillPageSize {
			break
		}
		lastID = missing[len(missing)-1].ID
	}

	// ponytail: GIN indexes for trigram search (ILIKE '%..%')
	indexes := []string{
		"CREATE EXTENSION IF NOT EXISTS pg_trgm",
		"CREATE INDEX IF NOT EXISTS idx_guest_records_name_trgm ON guest_records USING gin (name gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_guest_records_name_pinyin_trgm ON guest_records USING gin (name_pinyin gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_id ON guest_records (wedding_id, id)",
		"CREATE INDEX IF NOT EXISTS idx_guest_records_wedding_name ON guest_records (wedding_id, name)",
		// ponytail: FK lookup indexes
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users (email)",
		"CREATE INDEX IF NOT EXISTS idx_banquet_tables_wedding_name ON banquet_tables (wedding_id, name)",
		"CREATE INDEX IF NOT EXISTS idx_refresh_tokens_wedding_id ON refresh_tokens (wedding_id)",
		"CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at)",
	}
	for _, stmt := range indexes {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("migrate index: %w", err)
		}
	}

	// Expired refresh tokens are unusable; drop them so the table does not grow
	// forever. Keep a week for debugging.
	if err := db.Where("expires_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&models.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("delete expired refresh tokens: %w", err)
	}

	if db.Migrator().HasColumn(&models.HallElement{}, "label") {
		if err := db.Migrator().DropColumn(&models.HallElement{}, "label"); err != nil {
			return fmt.Errorf("drop legacy hall_elements.label: %w", err)
		}
	}
	// Seed default elements for weddings that have none — one query, not one
	// COUNT per wedding. The advisory lock above serializes the check-and-create,
	// so concurrent migrate runs cannot seed the same wedding twice.
	var missingHall []uuid.UUID
	if err := db.Model(&models.WeddingEvent{}).
		Where("NOT EXISTS (SELECT 1 FROM hall_elements WHERE hall_elements.wedding_id = wedding_events.id)").
		Pluck("id", &missingHall).Error; err != nil {
		return fmt.Errorf("find weddings missing hall elements: %w", err)
	}
	for _, wid := range missingHall {
		if err := db.Create(models.DefaultElements(wid)).Error; err != nil {
			return fmt.Errorf("seed default elements for wedding %s: %w", wid, err)
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// spaHandler serves the built frontend and falls back to index.html for client
// routes. The wildcard value is unescaped, so reject traversal before any
// filesystem access.
func spaHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Raw mux route: Fuego middleware does not run here, so set the
		// baseline headers by hand (uploads have their own, stronger set).
		middleware.SetSecurityHeaders(w)
		path := r.PathValue("path")
		if path == "" {
			path = "index.html"
		}
		if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
			http.NotFound(w, r)
			return
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
	}
}
