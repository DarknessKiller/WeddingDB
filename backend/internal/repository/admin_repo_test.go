package repository

import (
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdminUpdateFields_LeavesOtherColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`).Error)
	repo := NewAdminRepo(db)
	ctx := t.Context()
	id := uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, 'a@x.test', 'old-hash', 'user')`, id).Error)

	// A role change racing a password reset must not write the old hash back.
	testutil.NoErr(t, repo.UpdateFields(ctx, id, map[string]any{"password": "new-hash", "force_password_change": true}))
	testutil.NoErr(t, repo.UpdateFields(ctx, id, map[string]any{"role": "admin"}))

	var got models.AdminUser
	testutil.NoErr(t, db.First(&got, "id = ?", id).Error)
	if got.Password != "new-hash" || !got.ForcePasswordChange || got.Role != "admin" {
		t.Fatalf("got password=%q force=%v role=%q", got.Password, got.ForcePasswordChange, got.Role)
	}
}
