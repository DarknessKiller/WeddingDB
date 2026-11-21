package repository

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DeleteExpired removes only rows that expired before the cutoff.
func TestTokenRepo_DeleteExpiredHonoursCutoff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	now := time.Now()
	for token, exp := range map[string]time.Time{
		"old":    now.AddDate(0, 0, -8),
		"recent": now.AddDate(0, 0, -1),
		"live":   now.Add(time.Hour),
	} {
		testutil.NoErr(t, db.Create(&models.RefreshToken{ID: uuid.New(), AdminID: uuid.New(), Token: token, ExpiresAt: exp}).Error)
	}

	testutil.NoErr(t, NewTokenRepo(db).DeleteExpired(t.Context(), now.AddDate(0, 0, -7)))

	var left []string
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Order("token").Pluck("token", &left).Error)
	if len(left) != 2 || left[0] != "live" || left[1] != "recent" {
		t.Fatalf("tokens left = %v, want [live recent]", left)
	}
}

// A revoke that lands while a rotation is mid-transaction must also delete the
// successor token. The callback parks Rotate after it inserted the successor,
// so DeleteByAdminID always starts inside that window.
func TestTokenRepo_RevokeDuringRotateDeletesSuccessor(t *testing.T) {
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
	drop := func() { db.Exec("DROP TABLE IF EXISTS admin_users, refresh_tokens CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.Exec(adminUsersDDL).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id UUID PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id UUID,
		token TEXT NOT NULL UNIQUE, expires_at TIMESTAMPTZ, created_at TIMESTAMPTZ, rotated_at TIMESTAMPTZ, replaced_by TEXT)`).Error)
	adminID := seedAdmins(t, db, 1)[0]

	inserted, resume := make(chan struct{}), make(chan struct{})
	testutil.NoErr(t, db.Callback().Create().After("gorm:create").Register("test:park", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" && tx.Statement.Dest.(*models.RefreshToken).Token == HashToken("t2") {
			close(inserted)
			<-resume
		}
	}))
	repo := NewTokenRepo(db)
	exp := time.Now().Add(time.Hour)
	testutil.NoErr(t, repo.Save(t.Context(), &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "t1", ExpiresAt: exp}))

	rotated := make(chan error, 1)
	go func() {
		rotated <- repo.Rotate(t.Context(), "t1", &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "t2", ExpiresAt: exp})
	}()
	<-inserted
	revoked := make(chan error, 1)
	go func() { revoked <- repo.DeleteByAdminID(t.Context(), adminID, 0) }()
	time.Sleep(100 * time.Millisecond) // let the DELETE start and block before the rotation commits
	close(resume)
	testutil.NoErr(t, <-rotated)
	testutil.NoErr(t, <-revoked)

	var left []string
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Where("admin_id = ?", adminID).Pluck("token", &left).Error)
	if len(left) != 0 {
		t.Fatalf("tokens left after revoke = %v, want none", left)
	}
}

// Select-wedding must not rescope a token that was already rotated: the live
// successor would keep the old wedding and silently switch the user back.
func TestTokenRepo_UpdateWeddingScopeSkipsRotatedToken(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	adminID, now := uuid.New(), time.Now()
	testutil.NoErr(t, db.Create(&models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: HashToken("used"), ExpiresAt: now.Add(time.Hour), RotatedAt: &now}).Error)

	found, err := NewTokenRepo(db).UpdateWeddingScope(t.Context(), adminID, "used", uuid.New())
	testutil.NoErr(t, err)
	if found {
		t.Fatal("rescoped a rotated token")
	}
}

// The database holds only hashes: a leaked dump must not carry a usable
// refresh token, either as a live row or as a rotated row's successor.
func TestTokenRepo_StoresOnlyHashes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	repo := NewTokenRepo(db)
	adminID, exp := uuid.New(), time.Now().Add(time.Hour)
	testutil.NoErr(t, repo.Save(t.Context(), &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "raw-first", ExpiresAt: exp}))
	next := &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "raw-second", ExpiresAt: exp}
	testutil.NoErr(t, repo.Rotate(t.Context(), "raw-first", next))
	if next.Token != "raw-second" {
		t.Errorf("Rotate changed the caller's raw token to %q", next.Token)
	}

	var stored []string
	testutil.NoErr(t, db.Raw(`SELECT token FROM refresh_tokens UNION ALL
		SELECT replaced_by FROM refresh_tokens WHERE replaced_by IS NOT NULL`).Scan(&stored).Error)
	for _, v := range stored {
		if strings.HasPrefix(v, "raw-") {
			t.Errorf("raw refresh token stored: %q", v)
		}
	}
	if _, err := repo.FindByToken(t.Context(), "raw-second"); err != nil {
		t.Errorf("FindByToken(raw successor): %v", err)
	}

	// A row from before hashing holds the 64-char hex secret itself.
	legacy := strings.Repeat("ab", 32)
	testutil.NoErr(t, db.Create(&models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: legacy, ExpiresAt: exp}).Error)
	testutil.NoErr(t, repo.DeleteLegacyPlaintext(t.Context()))
	var left int64
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Where("token = ?", legacy).Count(&left).Error)
	if left != 0 {
		t.Error("legacy plaintext token survived DeleteLegacyPlaintext")
	}
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Count(&left).Error)
	if left != 2 {
		t.Errorf("rows left = %d, want the 2 hashed rows", left)
	}
}

// A deleted owner reads as no row, which Scan leaves at version 0: the token
// must not be stored for a user who no longer exists.
func TestTokenRepo_SaveAtVersionRefusesMissingOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, token_version INTEGER, deleted_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	err = NewTokenRepo(db).SaveAtVersion(t.Context(), &models.RefreshToken{ID: uuid.New(), AdminID: uuid.New(), Token: "t", ExpiresAt: time.Now().Add(time.Hour)}, 0)
	if !errors.Is(err, ErrTokenVersionStale) {
		t.Fatalf("err = %v, want ErrTokenVersionStale", err)
	}
}
