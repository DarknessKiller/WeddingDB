package repository

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The guard runs inside the write's transaction; Postgres adds the row lock
// that stops two concurrent removals from both passing.
func TestAdminRepo_LastAdminGuard(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT, password TEXT,
		name TEXT, role TEXT, force_password_change INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT)`).Error)
	a, b := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{a, b} {
		testutil.NoErr(t, db.Create(&models.AdminUser{ID: id, Email: id.String(), Role: "admin"}).Error)
	}
	repo := NewAdminRepo(db)
	ctx := context.Background()

	testutil.NoErr(t, repo.UpdateRole(ctx, a, "user"))
	if err := repo.UpdateRole(ctx, b, "user"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin err = %v, want ErrLastAdmin", err)
	}
	if err := repo.Delete(ctx, b); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin err = %v, want ErrLastAdmin", err)
	}
	testutil.NoErr(t, repo.Delete(ctx, a))
}
