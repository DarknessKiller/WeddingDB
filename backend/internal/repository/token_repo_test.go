package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Selecting a wedding must not report success when no live refresh token was
// rescoped: the caller would keep a session that forgets the wedding.
func TestTokenRepo_UpdateWeddingScopeMissingRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (
		id TEXT PRIMARY KEY, admin_id TEXT, wedding_id TEXT, token TEXT,
		expires_at DATETIME, created_at DATETIME, rotated_at DATETIME,
		replaced_by TEXT, token_version INTEGER)`).Error)
	repo := NewTokenRepo(db)
	ctx := context.Background()

	adminID := uuid.New()
	token := "live-token"
	testutil.NoErr(t, db.Create(&models.RefreshToken{
		ID:        uuid.New(),
		AdminID:   adminID,
		Token:     token,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	if err := repo.UpdateWeddingScope(ctx, adminID, "unknown-token", uuid.New()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update scope of unknown token err = %v, want gorm.ErrRecordNotFound", err)
	}

	wid := uuid.New()
	testutil.NoErr(t, repo.UpdateWeddingScope(ctx, adminID, token, wid))
	var got models.RefreshToken
	testutil.NoErr(t, db.First(&got, "token = ?", token).Error)
	if got.WeddingID == nil || *got.WeddingID != wid {
		t.Fatalf("wedding scope = %v, want %s", got.WeddingID, wid)
	}
}
