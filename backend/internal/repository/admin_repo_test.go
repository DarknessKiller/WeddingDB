package repository

import (
	"errors"
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

func TestLastAdminGuard(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT, created_at DATETIME)`).Error)
	repo := NewAdminRepo(db)
	ctx := t.Context()
	a, b, u := uuid.New(), uuid.New(), uuid.New()
	for _, row := range []struct {
		id   uuid.UUID
		role string
	}{{a, "admin"}, {b, "admin"}, {u, "user"}} {
		testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, 'h', ?)`, row.id, row.id.String(), row.role).Error)
	}

	testutil.NoErr(t, repo.UpdateRole(ctx, a, "user")) // b remains
	if err := repo.UpdateRole(ctx, b, "user"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: err = %v, want ErrLastAdmin", err)
	}
	if err := repo.Delete(ctx, b); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: err = %v, want ErrLastAdmin", err)
	}
	testutil.NoErr(t, repo.Delete(ctx, u)) // non-admins are never guarded
}
