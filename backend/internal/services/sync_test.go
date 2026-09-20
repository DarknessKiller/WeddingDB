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
		walk_in INTEGER DEFAULT 0,
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
		x REAL NOT NULL DEFAULT 0,
		y REAL NOT NULL DEFAULT 0,
		degree REAL NOT NULL DEFAULT 0,
		is_vip INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	guestRepo := repository.NewGuestRepo(db)
	tableRepo := repository.NewTableRepo(db)
	svc := NewGuestService(guestRepo, tableRepo, nil, db)
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

// TestSync_UnassignViaNullPointer covers fix 2: the app sends JSON null for
// tableId/seatNum when unassigning, so nil pointer AND empty string must both
// clear the assignment. SeatNum must never survive with a nil TableID.
func TestSync_UnassignViaNullPointer(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)

	seedUnassigned := func(name string) uuid.UUID {
		gid := uuid.New()
		tableID := uuid.New()
		seat := 2
		db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: name, Pax: 1, TableID: &tableID, SeatNum: &seat, CreatedAt: base, UpdatedAt: base})
		return gid
	}

	// nil pointer clears both table and seat
	gid := seedUnassigned("NullCase")
	res, _ := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: "NullCase", Pax: 1},
	}})
	if res[0].Status != "applied" {
		t.Fatalf("nil tableId update should apply, got %v", res[0])
	}
	g, _ := svc.Get(ctx, gid, wid)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("nil tableId should clear table and seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// empty string clears both too
	gid = seedUnassigned("EmptyCase")
	empty := ""
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: "EmptyCase", Pax: 1, TableID: &empty},
	}})
	if res[0].Status != "applied" {
		t.Fatalf("empty tableId update should apply, got %v", res[0])
	}
	g, _ = svc.Get(ctx, gid, wid)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("empty tableId should clear table and seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// a stray seatNum must not survive an unassign
	gid = seedUnassigned("StraySeat")
	seat := 3
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: "StraySeat", Pax: 1, SeatNum: &seat},
	}})
	if res[0].Status != "applied" {
		t.Fatalf("unassign with stray seatNum should apply, got %v", res[0])
	}
	g, _ = svc.Get(ctx, gid, wid)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("seat must not survive unassign, got table=%v seat=%v", g.TableID, g.SeatNum)
	}
}

// TestSync_DeleteLosesLWW_Failed covers fix 6: a delete that loses LWW must
// report "failed" (not "skipped") so the client keeps the delete queued
// instead of dequeuing it and losing the intent.
func TestSync_DeleteLosesLWW_Failed(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Doomed", Pax: 1, CreatedAt: base, UpdatedAt: base})

	// Older delete loses LWW -> failed, row must survive
	res, _ := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: base.Add(-time.Minute),
	}})
	if res[0].Status != "failed" {
		t.Fatalf("losing delete should be failed, got %v", res[0])
	}
	if _, err := svc.Get(ctx, gid, wid); err != nil {
		t.Fatalf("guest should still exist after losing delete: %v", err)
	}

	// Equal timestamps win the tie-break: the delete applies
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: base,
	}})
	if res[0].Status != "applied" {
		t.Fatalf("equal-timestamp delete should apply, got %v", res[0])
	}
	if _, err := svc.Get(ctx, gid, wid); err == nil {
		t.Fatal("guest should be deleted after winning delete")
	}
}

// TestSync_FutureClockClamped covers fix 3a: an op timestamped in the future
// is clamped to server time so a skewed client cannot win every future LWW.
func TestSync_FutureClockClamped(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Skewy", Pax: 1, CreatedAt: base, UpdatedAt: base})

	res, _ := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: time.Now().Add(time.Hour),
		Payload:         &SyncPayload{Name: "Skewy-new", Pax: 1},
	}})
	if res[0].Status != "applied" {
		t.Fatalf("future-dated update should still apply, got %v", res[0])
	}
	g, _ := svc.Get(ctx, gid, wid)
	if g.UpdatedAt.After(time.Now()) {
		t.Fatalf("record UpdatedAt must be clamped to server time, got %v", g.UpdatedAt)
	}
}

// TestSync_WriteError_Failed covers fix 1: a repo write failure must surface
// as "failed" so the client keeps (and retries) the mutation instead of
// dequeuing an op that never landed.
func TestSync_WriteError_Failed(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)
	// Same guest id already exists in a different wedding: the sync for wid
	// sees "not found" but the insert then collides on the primary key.
	otherWid := uuid.New()
	collideID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	db.Create(&models.GuestRecord{ID: collideID, WeddingID: otherWid, Name: "Collision", Pax: 1, CreatedAt: base, UpdatedAt: base})

	res, _ := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: collideID.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: "Collision", Pax: 1},
	}})
	if res[0].Status != "failed" {
		t.Fatalf("write error should report failed, got %v", res[0])
	}
	if res[0].Reason == "" {
		t.Fatal("failed result should carry a reason")
	}
}

// TestSync_TableSeatValidation covers fix 4: sync seat assignment validates
// the table (scoped to the wedding) and capacity instead of blindly writing.
func TestSync_TableSeatValidation(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)
	table := &models.BanquetTable{ID: uuid.New(), WeddingID: wid, Name: "T1", Capacity: 4}
	db.Create(table)
	missing := uuid.New()

	seed := func(name string) uuid.UUID {
		gid := uuid.New()
		db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: name, Pax: 1, CreatedAt: base, UpdatedAt: base})
		return gid
	}
	tID := table.ID.String()
	missingID := missing.String()

	// Valid assignment applies
	gid := seed("ValidSeat")
	seat := 3
	res, _ := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "ValidSeat", Pax: 1, TableID: &tID, SeatNum: &seat},
	}})
	if res[0].Status != "applied" {
		t.Fatalf("valid seat assign should apply, got %v", res[0])
	}
	g, _ := svc.Get(ctx, gid, wid)
	if g.TableID == nil || *g.TableID != table.ID || g.SeatNum == nil || *g.SeatNum != 3 {
		t.Fatalf("expected table+seat assigned, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// Unknown table -> failed
	gid = seed("GhostTable")
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "GhostTable", Pax: 1, TableID: &missingID, SeatNum: &seat},
	}})
	if res[0].Status != "failed" {
		t.Fatalf("unknown table should fail, got %v", res[0])
	}

	// Seat beyond capacity -> failed
	gid = seed("TooDeep")
	bigSeat := 5
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "TooDeep", Pax: 1, TableID: &tID, SeatNum: &bigSeat},
	}})
	if res[0].Status != "failed" {
		t.Fatalf("seat over capacity should fail, got %v", res[0])
	}

	// Create with unknown table -> failed too
	res, _ = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "FreshGhost", Pax: 1, TableID: &missingID, SeatNum: &seat},
	}})
	if res[0].Status != "failed" {
		t.Fatalf("create with unknown table should fail, got %v", res[0])
	}
}

// TestBulkCreate_RollsBackOnError covers fix 5: a bad row mid-batch must roll
// back the whole transaction so no partial import survives a retry.
func TestBulkCreate_RollsBackOnError(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	table := &models.BanquetTable{ID: uuid.New(), WeddingID: wid, Name: "T1", Capacity: 2}
	db.Create(table)
	missing := uuid.New()

	// First guest is fine, second references a missing table -> whole batch rolls back.
	seat := 1
	batch := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "Ok", Pax: 1},
		{ID: uuid.New(), WeddingID: wid, Name: "Bad", Pax: 1, TableID: &missing, SeatNum: &seat},
	}
	count, err := svc.BulkCreate(ctx, batch)
	if err == nil {
		t.Fatal("expected error from batch with missing table")
	}
	if count != 0 {
		t.Fatalf("rolled-back batch should report 0 created, got %d", count)
	}
	var total int64
	db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&total)
	if total != 0 {
		t.Fatalf("rolled-back batch must leave no rows, got %d", total)
	}

	// A clean batch still commits fully.
	clean := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "A", Pax: 1},
		{ID: uuid.New(), WeddingID: wid, Name: "B", Pax: 1},
	}
	count, err = svc.BulkCreate(ctx, clean)
	if err != nil || count != 2 {
		t.Fatalf("clean batch should create 2, got %d err %v", count, err)
	}
}
