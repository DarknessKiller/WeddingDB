package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConditionalCheckIn_MissingVsAlreadyCheckedIn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT, checked_in_at DATETIME,
		rsvp TEXT, notes TEXT, updated_at DATETIME)`).Error)
	repo := NewGuestRepo(db)
	ctx := context.Background()
	wid, gid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO guest_records (id, wedding_id) VALUES (?, ?)`, gid, wid).Error)

	if err := repo.ConditionalCheckIn(ctx, uuid.New(), wid, time.Now(), ""); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing guest: got %v, want gorm.ErrRecordNotFound", err)
	}
	testutil.NoErr(t, repo.ConditionalCheckIn(ctx, gid, wid, time.Now(), ""))
	if err := repo.ConditionalCheckIn(ctx, gid, wid, time.Now(), ""); !errors.Is(err, ErrAlreadyCheckedIn) {
		t.Fatalf("second check-in: got %v, want ErrAlreadyCheckedIn", err)
	}
}
