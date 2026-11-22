package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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
		field_times TEXT,
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
	svc := NewGuestService(guestRepo, nil, db)
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
	res, err := svc.Sync(ctx, wid, []SyncMutation{m1})
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("older update should be skipped, got %v", res[0])
	}
	// verify server unchanged
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
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
	res, err = svc.Sync(ctx, wid, []SyncMutation{m2})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("newer update should be applied, got %v", res[0])
	}
	g, err = svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
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
	res, err := svc.Sync(ctx, wid, []SyncMutation{m})
	testutil.NoErr(t, err)
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
	res, err := svc.Sync(ctx, wid, []SyncMutation{m1})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("checkin should apply")
	}
	// older checkin should be skipped (even though would overwrite checkedInAt)
	older := base.Add(1 * time.Minute)
	m2 := SyncMutation{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: older}
	res, err = svc.Sync(ctx, wid, []SyncMutation{m2})
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("older checkin should be skipped")
	}
	// checkout newer should win
	t2 := base.Add(10 * time.Minute)
	m3 := SyncMutation{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t2}
	res, err = svc.Sync(ctx, wid, []SyncMutation{m3})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("checkout should apply")
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt != nil {
		t.Fatalf("should be checked out")
	}
}

// TestSync_CheckInReplayDoesNotDuplicateNotes covers invariant 5: replaying a
// check-in after a checkout must not append its note a second time.
func TestSync_CheckInReplayDoesNotDuplicateNotes(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Replay", Pax: 1, CreatedAt: base, UpdatedAt: base})

	t1 := base.Add(time.Minute)
	var note SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"notes":"no gift"}`), &note))
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: t1, Payload: &note,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("first checkin should apply, got %v", res[0])
	}

	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: base.Add(2 * time.Minute),
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("checkout should apply, got %v", res[0])
	}

	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: t1, Payload: &note,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("replay should report applied, got %v", res[0])
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt != nil {
		t.Fatalf("replay must not re-check-in, got %v", g.CheckedInAt)
	}
	if g.Notes != "no gift" {
		t.Fatalf("replay duplicated the note: %q", g.Notes)
	}
}

// TestSync_UnassignViaNullPointer covers presence semantics: an absent tableId
// keeps the stored seat, while an explicit JSON null clears table and seat as
// one group. SeatNum must never survive with a nil TableID.
func TestSync_UnassignViaNullPointer(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)

	seedSeated := func(name string) uuid.UUID {
		gid := uuid.New()
		tableID := uuid.New()
		seat := 2
		db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "Seat-" + name, Capacity: 8})
		db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: name, Pax: 1, TableID: &tableID, SeatNum: &seat, CreatedAt: base, UpdatedAt: base})
		return gid
	}

	// Absent tableId preserves the seat assignment.
	gid := seedSeated("AbsentTable")
	var present SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"name":"AbsentTable","pax":1}`), &present))
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &present,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("sparse update should apply, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID == nil || g.SeatNum == nil {
		t.Fatalf("absent tableId must preserve the seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// Explicit JSON null clears both.
	var cleared SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"tableId":null,"seatNum":null}`), &cleared))
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(2 * time.Minute),
		Payload:         &cleared,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("null tableId update should apply, got %v", res[0])
	}
	g, err = svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("null tableId should clear table and seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// A stray seatNum without a tableId must not recreate a seat.
	var stray SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"seatNum":3}`), &stray))
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(3 * time.Minute),
		Payload:         &stray,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("seat-only patch must win no group, got %v", res[0])
	}
	g, err = svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("seat must not appear without a table, got table=%v seat=%v", g.TableID, g.SeatNum)
	}
}

// TestSync_PartialUpdatePreservesFields proves a newer edit to one column does
// not discard an older edit to a different column.
func TestSync_PartialUpdatePreservesFields(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Partial", Pax: 1, CreatedAt: base, UpdatedAt: base})

	var dietary, notes SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"dietary":["vegan"]}`), &dietary))
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &dietary,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("dietary update should apply, got %v", res[0])
	}

	testutil.NoErr(t, json.Unmarshal([]byte(`{"notes":"late note"}`), &notes))
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(2 * time.Minute),
		Payload:         &notes,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("notes update should apply, got %v", res[0])
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.Notes != "late note" {
		t.Fatalf("notes = %q, want late note", g.Notes)
	}
	if len(g.Dietary) != 1 || g.Dietary[0] != "vegan" {
		t.Fatalf("older dietary edit must survive, got %v", g.Dietary)
	}
}

// TestSync_TableAndSeatWinTogether proves an older table move cannot half-apply
// its table or its seat.
func TestSync_TableAndSeatWinTogether(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)
	newTable := &models.BanquetTable{ID: uuid.New(), WeddingID: wid, Name: "T1", Capacity: 10}
	db.Create(newTable)
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "SeatMove", Pax: 1, CreatedAt: base, UpdatedAt: base})

	newer := newTable.ID.String()
	var win SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"tableId":"`+newer+`","seatNum":4}`), &win))
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(2 * time.Minute),
		Payload:         &win,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("newer table+seat should apply, got %v", res[0])
	}

	older := uuid.New().String()
	var lose SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"tableId":"`+older+`","seatNum":7}`), &lose))
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &lose,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("older table+seat should be skipped, got %v", res[0])
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID == nil || *g.TableID != newTable.ID || g.SeatNum == nil || *g.SeatNum != 4 {
		t.Fatalf("stale group must not half-apply: table=%v seat=%v", g.TableID, g.SeatNum)
	}
}

// TestSync_UndoBeatsUnrelatedNewerEdit proves checkout keeps the row checked
// out even when a newer edit touches a different column.
func TestSync_UndoBeatsUnrelatedNewerEdit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	checkedIn := base
	db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Undo", Pax: 1,
		CheckedInAt: &checkedIn, CreatedAt: base, UpdatedAt: base,
	})

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("checkout should apply, got %v", res[0])
	}

	var notes SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"notes":"newer note"}`), &notes))
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(2 * time.Minute),
		Payload:         &notes,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("unrelated edit should apply, got %v", res[0])
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt != nil {
		t.Fatalf("unrelated newer edit reverted the checkout: %v", g.CheckedInAt)
	}
	if g.Notes != "newer note" {
		t.Fatalf("notes = %q, want newer note", g.Notes)
	}
}

// TestSync_ZeroClientTimeFails rejects a mutation that carries no timestamp.
func TestSync_ZeroClientTimeFails(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "NoClock", Pax: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()})

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		Payload: &SyncPayload{Name: "NoClock"},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" || res[0].Reason != "clientUpdatedAt required" {
		t.Fatalf("zero client time should fail with the required reason, got %v", res[0])
	}
}

// TestUpdatePatch_PreservesAbsentFields proves a sparse online PUT keeps every
// field the client did not send.
func TestUpdatePatch_PreservesAbsentFields(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	table := &models.BanquetTable{ID: uuid.New(), WeddingID: wid, Name: "T1", Capacity: 10}
	db.Create(table)
	gid := uuid.New()
	seat := 2
	db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Sparse", Email: "keep@example.com", Pax: 1,
		TableID: &table.ID, SeatNum: &seat, Notes: "keep note", Dietary: []string{"fish"}, IsVip: true,
	})

	var patch GuestPatch
	testutil.NoErr(t, json.Unmarshal([]byte(`{"pax":3}`), &patch))
	merged, err := svc.UpdatePatch(ctx, gid, wid, &patch)
	testutil.NoErr(t, err)
	if merged.Pax != 3 {
		t.Fatalf("pax = %d, want 3", merged.Pax)
	}
	if merged.Notes != "keep note" || merged.Email != "keep@example.com" || !merged.IsVip {
		t.Fatalf("absent fields were not preserved: %+v", merged)
	}
	if merged.TableID == nil || *merged.TableID != table.ID || merged.SeatNum == nil || *merged.SeatNum != seat {
		t.Fatalf("seat was not preserved: table=%v seat=%v", merged.TableID, merged.SeatNum)
	}
	if len(merged.Dietary) != 1 || merged.Dietary[0] != "fish" {
		t.Fatalf("dietary = %v, want fish", merged.Dietary)
	}
}

// report "failed" (not "skipped") so the client keeps the delete queued
// instead of dequeuing it and losing the intent.
func TestSync_DeleteLosesLWW_Failed(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Doomed", Pax: 1, CreatedAt: base, UpdatedAt: base})

	// Older delete loses LWW -> failed, row must survive
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: base.Add(-time.Minute),
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("losing delete should be failed, got %v", res[0])
	}
	if _, err := svc.Get(ctx, gid, wid); err != nil {
		t.Fatalf("guest should still exist after losing delete: %v", err)
	}

	// Equal timestamps win the tie-break: the delete applies
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: base,
	}})
	testutil.NoErr(t, err)
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

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: time.Now().Add(time.Hour),
		Payload:         &SyncPayload{Name: "Skewy-new", Pax: 1},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("future-dated update should still apply, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.UpdatedAt.After(time.Now()) {
		t.Fatalf("record UpdatedAt must be clamped to server time, got %v", g.UpdatedAt)
	}
	if g.FieldTimes[groupName].After(time.Now()) {
		t.Fatalf("stored field time must be the clamped time, got %v", g.FieldTimes[groupName])
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

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: collideID.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: "Collision", Pax: 1},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("write error should report failed, got %v", res[0])
	}
	if res[0].Reason == "" {
		t.Fatal("failed result should carry a reason")
	}
}

func TestSyncDatabaseErrorIsGeneric(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	testutil.NoErr(t, db.Exec("DROP TABLE guest_records").Error)
	results, err := svc.Sync(context.Background(), wid, []SyncMutation{
		{Op: SyncOpCreate, GuestID: uuid.NewString(), ClientUpdatedAt: time.Now(), Payload: &SyncPayload{Name: "Test"}},
		{Op: "unknown", GuestID: uuid.NewString(), ClientUpdatedAt: time.Now()},
	})
	testutil.NoErr(t, err)
	if results[0].Status != "failed" || results[0].Reason != "Internal error" || results[1].Status != "skipped" {
		t.Fatalf("results = %+v", results)
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
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "ValidSeat", Pax: 1, TableID: &tID, SeatNum: &seat},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("valid seat assign should apply, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID == nil || *g.TableID != table.ID || g.SeatNum == nil || *g.SeatNum != 3 {
		t.Fatalf("expected table+seat assigned, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// Unknown table -> failed
	gid = seed("GhostTable")
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "GhostTable", Pax: 1, TableID: &missingID, SeatNum: &seat},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("unknown table should fail, got %v", res[0])
	}

	// Seat beyond capacity -> failed
	gid = seed("TooDeep")
	bigSeat := 5
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "TooDeep", Pax: 1, TableID: &tID, SeatNum: &bigSeat},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("seat over capacity should fail, got %v", res[0])
	}

	// Create with unknown table -> failed too
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: "FreshGhost", Pax: 1, TableID: &missingID, SeatNum: &seat},
	}})
	testutil.NoErr(t, err)
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

func TestCreate_RejectsOversizedParty(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	err := svc.Create(context.Background(), &models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Big", Pax: MaxGuestPartySize + 1,
	})
	if err == nil {
		t.Fatal("expected oversized party size to be rejected")
	}
}

// Offline sync invariant 7: a negative angbao amount fails online and offline.
func TestSync_NegativeAngbaoRejected(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	base := time.Now().Add(-10 * time.Minute)
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Gift", Pax: 1, CreatedAt: base, UpdatedAt: base})

	var patch SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"angbaoAmt":-5}`), &patch))
	if _, err := svc.UpdatePatch(ctx, gid, wid, &patch); !errors.Is(err, ErrGuestPatchInvalid) {
		t.Fatalf("online err = %v, want ErrGuestPatchInvalid", err)
	}
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute), Payload: &patch,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("offline result = %+v, want failed", res[0])
	}
}
