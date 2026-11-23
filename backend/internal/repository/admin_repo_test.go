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

// A role write that matched no row is a deleted user, not a successful update.
func TestAdminRepo_UpdateRoleDeletedIsNotFound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT, password TEXT,
		name TEXT, role TEXT, force_password_change INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	other := uuid.New()
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: other, Email: other.String(), Role: "admin"}).Error)
	repo := NewAdminRepo(db)

	err = repo.UpdateRole(context.Background(), uuid.New(), "user")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update deleted account err = %v, want gorm.ErrRecordNotFound", err)
	}
}

// A password write must not carry the rest of a stale struct back to the row:
// the demoted account stays demoted, and a deleted account stays deleted
// instead of being re-inserted.
func TestAdminRepo_UpdateWritesCredentialsOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT, password TEXT,
		name TEXT, role TEXT, force_password_change INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT)`).Error)
	other, id := uuid.New(), uuid.New()
	for _, admin := range []models.AdminUser{
		{ID: other, Email: other.String(), Role: "admin", Password: "x"},
		{ID: id, Email: id.String(), Role: "admin", Password: "old"},
	} {
		testutil.NoErr(t, db.Create(&admin).Error)
	}
	repo := NewAdminRepo(db)
	ctx := context.Background()

	stale := models.AdminUser{ID: id, Email: id.String(), Role: "admin", Password: "new", ForcePasswordChange: true}
	testutil.NoErr(t, repo.UpdateRole(ctx, id, "user"))
	testutil.NoErr(t, repo.Update(ctx, &stale))

	var got models.AdminUser
	testutil.NoErr(t, db.First(&got, "id = ?", id).Error)
	if got.Role != "user" {
		t.Fatalf("role = %q, want user (stale row must not restore it)", got.Role)
	}
	if got.Password != "new" || !got.ForcePasswordChange {
		t.Fatalf("credentials = %q/%v, want new/true", got.Password, got.ForcePasswordChange)
	}

	testutil.NoErr(t, repo.Delete(ctx, id))
	if err := repo.Update(ctx, &stale); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update deleted account err = %v, want gorm.ErrRecordNotFound", err)
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.AdminUser{}).Where("id = ?", id).Count(&count).Error)
	if count != 0 {
		t.Fatalf("deleted account re-inserted: %d rows", count)
	}
}
