package services

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

// newIntegrationAuthService creates an AuthService backed by in-memory SQLite + miniredis
// for testing flows that touch both Postgres (refresh tokens, admin lookup) and Redis (blacklist/tv).
func newIntegrationAuthService(t *testing.T, secret string) (*AuthService, *miniredis.Miniredis, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Create tables manually since gen_random_uuid() is Postgres-specific
	db.Exec(`CREATE TABLE refresh_tokens (
		id TEXT PRIMARY KEY,
		admin_id TEXT NOT NULL,
		wedding_id TEXT,
		token TEXT NOT NULL UNIQUE,
		expires_at DATETIME,
		created_at DATETIME
	)`)
	db.Exec(`CREATE INDEX idx_refresh_tokens_admin_id ON refresh_tokens(admin_id)`)

	db.Exec(`CREATE TABLE admin_users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		password TEXT NOT NULL,
		name TEXT,
		role TEXT NOT NULL,
		force_password_change INTEGER DEFAULT 0,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE UNIQUE INDEX idx_admin_users_email ON admin_users(email)`)

	db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		date DATETIME,
		kiosk_title TEXT,
		kiosk_description TEXT,
		kiosk_logo_url TEXT,
		kiosk_background_url TEXT,
		kiosk_background_blur INTEGER DEFAULT 0,
		kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT,
		kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT,
		kiosk_logo_pos_x TEXT,
		kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER DEFAULT 1,
		hall_width INTEGER DEFAULT 860,
		hall_height INTEGER DEFAULT 1000,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	tokenRepo := repository.NewTokenRepo(db)
	adminRepo := repository.NewAdminRepo(db)
	weddingRepo := repository.NewWeddingRepo(db)
	svc := &AuthService{secret: []byte(secret), redisClient: rdb, tokenRepo: tokenRepo, adminRepo: adminRepo, weddingRepo: weddingRepo}
	return svc, mr, db
}

// seedAdmin inserts an admin user into the test database.
func seedAdmin(t *testing.T, db *gorm.DB, adminID uuid.UUID, role string) {
	t.Helper()
	admin := &models.AdminUser{
		ID:       adminID,
		Email:    adminID.String() + "@test.com",
		Password: "hashed-password",
		Name:     "Test User",
		Role:     role,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("failed to seed admin: %v", err)
	}
}

// seedRefreshToken inserts a refresh token.
func seedRefreshToken(t *testing.T, db *gorm.DB, adminID uuid.UUID, weddingID *uuid.UUID, tokenStr string) {
	t.Helper()
	rt := &models.RefreshToken{
		ID:        uuid.New(),
		AdminID:   adminID,
		WeddingID: weddingID,
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := db.Create(rt).Error; err != nil {
		t.Fatalf("failed to seed refresh token: %v", err)
	}
}

// --- Logout blacklisting ---

func TestLogout_BlacklistsAccessToken(t *testing.T) {
	svc, mr, _ := newIntegrationAuthService(t, "test-secret")
	jti := uuid.New().String()
	adminID := uuid.New()

	tok := signTestToken("test-secret", AccessClaims{
		AdminID: adminID,
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})

	if err := svc.Logout(context.Background(), "dummy-refresh", tok); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	key := "blacklist:jti:" + jti
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected blacklist key %q to exist, got error: %v", key, err)
	}
	if val != "1" {
		t.Errorf("expected blacklist value '1', got %q", val)
	}

	ttl := mr.TTL(key)
	if ttl <= 0 {
		t.Errorf("expected positive TTL on blacklist key, got %v", ttl)
	}
}

func TestLogout_BlacklistedTokenRejected(t *testing.T) {
	svc, _, _ := newIntegrationAuthService(t, "test-secret")
	jti := uuid.New().String()
	adminID := uuid.New()

	tok := signTestToken("test-secret", AccessClaims{
		AdminID: adminID,
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})

	if err := svc.Logout(context.Background(), "dummy-refresh", tok); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected ValidateToken to reject blacklisted token after Logout, got nil")
	}
}

// --- Refresh rotation blacklisting ---

func TestRefresh_BlacklistsOldAccessToken(t *testing.T) {
	svc, mr, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	wid := uuid.New()
	oldJTI := uuid.New().String()
	refreshStr := "test-refresh-token-abc"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, &wid, refreshStr)

	oldTok := signTestToken("test-secret", AccessClaims{
		AdminID:   adminID,
		WeddingID: &wid,
		Role:      "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        oldJTI,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	})

	result, err := svc.Refresh(context.Background(), refreshStr, oldTok)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if result == nil || result.AccessToken == "" {
		t.Fatal("expected new access token from Refresh")
	}

	// Verify old JTI is blacklisted
	key := "blacklist:jti:" + oldJTI
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected old access token JTI to be blacklisted, got error: %v", err)
	}
	if val != "1" {
		t.Errorf("expected blacklist value '1', got %q", val)
	}

	// Verify old access token is rejected by ValidateToken
	_, err = svc.ValidateToken(context.Background(), oldTok)
	if err == nil {
		t.Fatal("expected old access token to be rejected after Refresh")
	}
}

func TestRefresh_WithoutOldAccessToken_NoBlacklist(t *testing.T) {
	svc, mr, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	wid := uuid.New()
	refreshStr := "test-refresh-token-def"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, &wid, refreshStr)

	result, err := svc.Refresh(context.Background(), refreshStr, "")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if result == nil || result.AccessToken == "" {
		t.Fatal("expected new access token")
	}

	// No blacklist keys should have been written
	keys := mr.Keys()
	for _, k := range keys {
		if len(k) > 10 && k[:10] == "blacklist:j" {
			t.Errorf("unexpected blacklist key written when no old access token provided: %q", k)
		}
	}
}

// --- RevokeUserTokens (delete before INCR TOCTOU) ---

func TestRevokeUserTokens_DeletesRefreshTokensAndBumpsTV(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	wid := uuid.New()

	seedRefreshToken(t, db, adminID, &wid, "token-1")
	seedRefreshToken(t, db, adminID, &wid, "token-2")

	var count int64
	db.Model(&models.RefreshToken{}).Where("admin_id = ?", adminID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 refresh tokens, got %d", count)
	}

	if err := svc.RevokeUserTokens(context.Background(), adminID); err != nil {
		t.Fatalf("RevokeUserTokens failed: %v", err)
	}

	db.Model(&models.RefreshToken{}).Where("admin_id = ?", adminID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 refresh tokens after revoke, got %d", count)
	}

	tv, err := svc.GetTokenVersion(context.Background(), adminID)
	if err != nil {
		t.Fatalf("GetTokenVersion failed: %v", err)
	}
	if tv != 1 {
		t.Errorf("expected token_version=1 after revoke, got %d", tv)
	}
}

func TestRevokeUserTokens_InvalidatesExistingTokens(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	wid := uuid.New()

	tok := signTestToken("test-secret", AccessClaims{
		AdminID:      adminID,
		WeddingID:    &wid,
		Role:         "admin",
		TokenVersion: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})

	_, err := svc.ValidateToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("token should be valid before revoke: %v", err)
	}

	seedRefreshToken(t, db, adminID, &wid, "seed-token")
	if err := svc.RevokeUserTokens(context.Background(), adminID); err != nil {
		t.Fatalf("RevokeUserTokens failed: %v", err)
	}

	_, err = svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected old token to be rejected after RevokeUserTokens bumped tv")
	}
}

func TestRevokeUserTokens_DeleteBeforeIncrOrdering(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()

	seedRefreshToken(t, db, adminID, nil, "orphan-token")

	tv, _ := svc.GetTokenVersion(context.Background(), adminID)
	if tv != 0 {
		t.Fatalf("expected initial tv=0, got %d", tv)
	}

	if err := svc.RevokeUserTokens(context.Background(), adminID); err != nil {
		t.Fatalf("RevokeUserTokens failed: %v", err)
	}

	var count int64
	db.Model(&models.RefreshToken{}).Where("admin_id = ?", adminID).Count(&count)
	if count != 0 {
		t.Errorf("refresh tokens should be deleted (checked first in RevokeUserTokens)")
	}

	tv, _ = svc.GetTokenVersion(context.Background(), adminID)
	if tv != 1 {
		t.Errorf("token version should be 1 after revoke, got %d", tv)
	}
}
