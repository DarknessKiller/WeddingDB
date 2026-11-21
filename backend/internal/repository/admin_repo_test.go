package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const adminUsersDDL = `CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT, password TEXT, name TEXT,
	role TEXT, force_password_change BOOLEAN, token_version INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMP, updated_at TIMESTAMP)`

func seedAdmins(t *testing.T, db *gorm.DB, n int) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
		testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, 'x', 'admin')`,
			ids[i], ids[i].String()+"@example.test").Error)
	}
	return ids
}

func TestAdminRepo_KeepsLastAdmin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(adminUsersDDL).Error)
	repo := NewAdminRepo(db)
	ctx := context.Background()
	ids := seedAdmins(t, db, 3)

	testutil.NoErr(t, repo.UpdateRole(ctx, ids[0], "user"))
	testutil.NoErr(t, repo.Delete(ctx, ids[1]))
	if err := repo.UpdateRole(ctx, ids[2], "user"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: got %v, want ErrLastAdmin", err)
	}
	if err := repo.Delete(ctx, ids[2]); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: got %v, want ErrLastAdmin", err)
	}
	// A non-admin is not guarded by the admin count.
	testutil.NoErr(t, repo.Delete(ctx, ids[0]))
}

// TestAdminRepo_ConcurrentDemoteKeepsOneAdmin proves the row lock: two admins
// demoting each other at once must leave one admin. Requires real Postgres.
func TestAdminRepo_ConcurrentDemoteKeepsOneAdmin(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() { db.Exec("DROP TABLE IF EXISTS admin_users CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.Exec(adminUsersDDL).Error)
	repo := NewAdminRepo(db)

	for range 20 {
		testutil.NoErr(t, db.Exec("DELETE FROM admin_users").Error)
		ids := seedAdmins(t, db, 2)
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Go(func() {
				if err := repo.UpdateRole(context.Background(), id, "user"); err != nil && !errors.Is(err, ErrLastAdmin) {
					t.Errorf("demote: %v", err)
				}
			})
		}
		wg.Wait()
		var admins int64
		testutil.NoErr(t, db.Model(&models.AdminUser{}).Where("role = ?", "admin").Count(&admins).Error)
		if admins != 1 {
			t.Fatalf("admins left = %d, want 1", admins)
		}
	}
}

// A password write must not carry a stale copy of the rest of the row: a role
// change made after the handler loaded the user has to survive it.
func TestAdminRepo_SetPasswordKeepsConcurrentRoleChange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(adminUsersDDL).Error)
	id := seedAdmins(t, db, 2)[0] // two admins, so demoting one is allowed
	repo := NewAdminRepo(db)

	testutil.NoErr(t, repo.UpdateRole(t.Context(), id, "user")) // lands between load and password write
	testutil.NoErr(t, repo.SetPassword(t.Context(), id, "new-hash", true))

	got, err := repo.FindByID(t.Context(), id)
	testutil.NoErr(t, err)
	if got.Role != "user" || got.Password != "new-hash" || !got.ForcePasswordChange {
		t.Fatalf("role=%q password=%q force=%v, want user/new-hash/true", got.Role, got.Password, got.ForcePasswordChange)
	}
}
