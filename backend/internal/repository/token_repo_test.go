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

// Logging out with a rotated token also deletes every successor it was rotated
// into; other sessions stay. A rotated token cannot be rescoped.
func TestTokenRepo_DeleteChainFollowsSuccessors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (
		id TEXT PRIMARY KEY, admin_id TEXT, wedding_id TEXT, token TEXT,
		expires_at DATETIME, created_at DATETIME, rotated_at DATETIME,
		replaced_by TEXT, token_version INTEGER)`).Error)
	repo := NewTokenRepo(db)
	ctx := t.Context()
	adminID := uuid.New()
	live := func(tok string) *models.RefreshToken {
		return &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: tok, ExpiresAt: time.Now().Add(time.Hour)}
	}
	testutil.NoErr(t, db.Create(live("t0")).Error)
	testutil.NoErr(t, db.Create(live("other")).Error)
	testutil.NoErr(t, repo.Rotate(ctx, "t0", live("t1")))
	testutil.NoErr(t, repo.Rotate(ctx, "t1", live("t2")))

	if err := repo.UpdateWeddingScope(ctx, adminID, "t0", uuid.New()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("rescope rotated token err = %v, want gorm.ErrRecordNotFound", err)
	}
	testutil.NoErr(t, repo.DeleteChain(ctx, "t0"))
	var left []string
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Order("token").Pluck("token", &left).Error)
	if len(left) != 1 || left[0] != "other" {
		t.Fatalf("tokens left = %v, want [other]", left)
	}
}
