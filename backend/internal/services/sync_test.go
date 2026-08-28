package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

func newGuestSyncService(t *testing.T) (*GuestService, *gorm.DB, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	db.Exec(`CREATE TABLE guest_records (
		id TEXT PRIMARY KEY,
		wedding_id TEXT NOT NULL,
		name TEXT NOT NULL,
		name_pinyin TEXT,
		phone TEXT,
		email TEXT,
		pax INTEGER NOT NULL DEFAULT 1,
		table_id TEXT,
		seat_num INTEGER,
		rsvp TEXT DEFAULT 'no_response',
		checked_in_at DATETIME,
		notes TEXT,
		dietary TEXT,
		is_vip INTEGER,
		angbao_amt INTEGER,
		gift_item TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE banquet_tables (
		id TEXT PRIMARY KEY,
		wedding_id TEXT NOT NULL,
		name TEXT NOT NULL,
		capacity INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	guestRepo := repository.NewGuestRepo(db)
	tableRepo := repository.NewTableRepo(db)
	svc := NewGuestService(guestRepo, tableRepo, nil)
	wid := uuid.New()
	return svc, db, wid
}

func TestSync_NewestWins(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	// seed server guest with newer time
	seed := &models.GuestRecord{
		ID:        gid,
		WeddingID: wid,
		Name:      "Alice",
		Pax:       1,
		CreatedAt: base.Add(5 * time.Minute),
		UpdatedAt: base.Add(5 * time.Minute),
	}
	// direct create via repo bypass
	db.Create(seed)

	// try update with older client time -> should be skipped
	older := base
	m1 := SyncMutation{
		Op:              SyncOpUpdate,
		GuestID:         gid.String(),
		ClientUpdatedAt: older,
		Payload:         &SyncPayload{Name: "Alice-old", Pax: 1},
	}
	res, _ := svc.Sync(ctx, wid, []SyncMutation{m1})
	if res[0].Status != "skipped" {
		t.Fatalf("older update should be skipped, got %v", res[0])
	}
	// verify server unchanged
	g, _ := svc.Get(ctx, gid, wid)
	if g.Name != "Alice" {
		t.Fatalf("name should stay Alice, got %q", g.Name)
	}

	// newer update -> applied
	newer := base.Add(10 * time.Minute)
	m2 := SyncMutation{
		Op:              SyncOpUpdate,
		GuestID:         gid.String(),
		ClientUpdatedAt: newer,
		Payload:         &SyncPayload{Name: "Alice-new", Pax: 2},
	}
	res, _ = svc.Sync(ctx, wid, []SyncMutation{m2})
	if res[0].Status != "applied" {
		t.Fatalf("newer update should be applied, got %v", res[0])
	}
	g, _ = svc.Get(ctx, gid, wid)
	if g.Name != "Alice-new" || g.Pax != 2 {
		t.Fatalf("expected Alice-new pax2, got %q pax %d", g.Name, g.Pax)
	}
}

func TestSync_Create(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	now := time.Now()
	m := SyncMutation{
		Op:              SyncOpCreate,
		GuestID:         gid.String(),
		ClientUpdatedAt: now,
		Payload:         &SyncPayload{Name: "Bob", Pax: 1},
	}
	res, _ := svc.Sync(ctx, wid, []SyncMutation{m})
	if res[0].Status != "applied" {
		t.Fatalf("create should be applied, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	if err != nil || g.Name != "Bob" {
		t.Fatalf("get after create failed: %v name %q", err, g.Name)
	}
}

func TestSync_CheckInLWW(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	seed := &models.GuestRecord{
		ID:        gid,
		WeddingID: wid,
		Name:      "Check",
		Pax:       1,
		CreatedAt: base,
		UpdatedAt: base,
	}
	db.Create(seed)
	// first checkin newer
	t1 := base.Add(5 * time.Minute)
	m1 := SyncMutation{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: t1}
	res, _ := svc.Sync(ctx, wid, []SyncMutation{m1})
	if res[0].Status != "applied" {
		t.Fatalf("checkin should apply")
	}
	// older checkin should be skipped (even though would overwrite checkedInAt)
	older := base.Add(1 * time.Minute)
	m2 := SyncMutation{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: older}
	res, _ = svc.Sync(ctx, wid, []SyncMutation{m2})
	if res[0].Status != "skipped" {
		t.Fatalf("older checkin should be skipped")
	}
	// checkout newer should win
	t2 := base.Add(10 * time.Minute)
	m3 := SyncMutation{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t2}
	res, _ = svc.Sync(ctx, wid, []SyncMutation{m3})
	if res[0].Status != "applied" {
		t.Fatalf("checkout should apply")
	}
	g, _ := svc.Get(ctx, gid, wid)
	if g.CheckedInAt != nil {
		t.Fatalf("should be checked out")
	}
}
