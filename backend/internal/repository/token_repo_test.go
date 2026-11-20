package repository

import (
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
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
