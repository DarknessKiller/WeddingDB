package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	testutil.NoErr(t, db.AutoMigrate(&models.GuestTombstone{}))
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

func syncStr(s string) *string { return &s }
func syncInt(n int) *int       { return &n }

// syncTable marks tableId as present in the payload; pass "" to unassign.
func syncTable(id string) NullableJSON[string] {
	return NullableJSON[string]{Set: true, Value: &id}
}

// syncTableNull marks an explicit JSON null tableId.
func syncTableNull() NullableJSON[string] {
	return NullableJSON[string]{Set: true}
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
		Payload:         &SyncPayload{Name: syncStr("Alice-old"), Pax: syncInt(1)},
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
		Payload:         &SyncPayload{Name: syncStr("Alice-new"), Pax: syncInt(2)},
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
		Payload:         &SyncPayload{Name: syncStr("Bob"), Pax: syncInt(1)},
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

// TestSync_OfflineCheckInSurvivesNewerEdit: a check-in queued offline must
// apply even when an online edit bumped updated_at after it was recorded.
func TestSync_OfflineCheckInSurvivesNewerEdit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour).UTC()
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Late", Pax: 1, CreatedAt: base, UpdatedAt: base.Add(5 * time.Minute)})

	amt := 88
	m := SyncMutation{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: base, Payload: &SyncPayload{AngbaoAmt: NullableJSON[int]{Set: true, Value: &amt}}}
	res, err := svc.Sync(ctx, wid, []SyncMutation{m})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("offline checkin should apply, got %s (%s)", res[0].Status, res[0].Reason)
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt == nil || g.AngbaoAmt == nil || *g.AngbaoAmt != amt {
		t.Fatalf("checkin or angbao lost: %+v", g)
	}
	// Forward, never backwards: an editor holding the 5-minute version must
	// now get a 409 instead of writing over this angbao.
	if !g.UpdatedAt.After(base.Add(5 * time.Minute)) {
		t.Fatalf("updated_at = %v, want after %v", g.UpdatedAt, base.Add(5*time.Minute))
	}
}

// An edit queued without a base after an edit of the same guest that was
// refused as stale was built on the refused edit: it must not apply and undo
// the change that made the first one stale.
func TestSync_BaselessEditAfterStaleRefusalFails(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	amt := 200
	// Another device keyed the angbao online at t1+5m.
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, AngbaoAmt: &amt, CreatedAt: t1, UpdatedAt: t1.Add(5 * time.Minute)})

	cleared := NullableJSON[int]{Set: true}
	res, err := svc.Sync(ctx, wid, []SyncMutation{
		{Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: t1.Add(2 * time.Minute), BaseUpdatedAt: &t1, Payload: &SyncPayload{Name: syncStr("X1"), AngbaoAmt: cleared}},
		{Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: t1.Add(6 * time.Minute), Payload: &SyncPayload{Name: syncStr("X2"), AngbaoAmt: cleared}},
	})
	testutil.NoErr(t, err)
	if res[0].Status == "applied" || res[1].Status != "failed" {
		t.Fatalf("statuses = %s, %s; want not applied, failed", res[0].Status, res[1].Status)
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.AngbaoAmt == nil || *g.AngbaoAmt != amt {
		t.Fatalf("angbao = %v, want %d kept", g.AngbaoAmt, amt)
	}
}

// A sync whose response was lost is sent again: every edit in it already
// applied, so the replay must report applied, not a stale failure.
func TestSync_ReplayOfAppliedBatchIsApplied(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid, gid2 := uuid.New(), uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, CreatedAt: t1, UpdatedAt: t1})
	db.Create(&models.GuestRecord{ID: gid2, WeddingID: wid, Name: "Z", Pax: 1, CreatedAt: t1, UpdatedAt: t1})

	amt := 88
	batch := []SyncMutation{
		{Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute), BaseUpdatedAt: &t1, Payload: &SyncPayload{Name: syncStr("Y")}},
		{Op: SyncOpCheckIn, GuestID: gid2.String(), ClientUpdatedAt: t1.Add(2 * time.Minute), Payload: &SyncPayload{AngbaoAmt: NullableJSON[int]{Set: true, Value: &amt}}},
	}
	for round := range 2 {
		res, err := svc.Sync(ctx, wid, batch)
		testutil.NoErr(t, err)
		for i, r := range res {
			if r.Status != "applied" {
				t.Fatalf("round %d mutation %d: %s (%s), want applied", round, i, r.Status, r.Reason)
			}
		}
	}
}

// TestSync_UnassignViaNullPointer covers the seat contract: an explicit JSON
// null (or empty string) tableId clears the assignment, while an absent
// tableId leaves the current seat untouched.
func TestSync_UnassignViaNullPointer(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)

	seedSeated := func(name string) uuid.UUID {
		gid := uuid.New()
		tableID := uuid.New()
		seat := 2
		db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "T", Capacity: 10})
		db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: name, Pax: 1, TableID: &tableID, SeatNum: &seat, CreatedAt: base, UpdatedAt: base})
		return gid
	}

	// explicit null clears both table and seat
	gid := seedSeated("NullCase")
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: syncStr("NullCase"), Pax: syncInt(1), TableID: syncTableNull()},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("null tableId update should apply, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("null tableId should clear table and seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// empty string clears both too
	gid = seedSeated("EmptyCase")
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: syncStr("EmptyCase"), Pax: syncInt(1), TableID: syncTable("")},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("empty tableId update should apply, got %v", res[0])
	}
	g, err = svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("empty tableId should clear table and seat, got table=%v seat=%v", g.TableID, g.SeatNum)
	}

	// an absent tableId leaves the existing seat alone
	gid = seedSeated("KeepSeat")
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Name: syncStr("KeepSeat"), Pax: syncInt(1)},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("seat-preserving update should apply, got %v", res[0])
	}
	g, err = svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.SeatNum == nil || *g.SeatNum != 2 {
		t.Fatalf("absent tableId must keep seat 2, got seat=%v", g.SeatNum)
	}
}

// TestSync_PartialUpdatePreservesFields covers the sync root cause: a mutation
// that sends only the fields it changes (e.g. a check-in note) must not wipe
// email, angbao, gift, or seat.
func TestSync_PartialUpdatePreservesFields(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-10 * time.Minute)
	tableID := uuid.New()
	db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "T", Capacity: 10})
	seat := 4
	angbao := 888
	gift := "Teapot"
	gid := uuid.New()
	db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Full", Pax: 2, Email: "a@example.com",
		AngbaoAmt: &angbao, GiftItem: &gift,
		TableID: &tableID, SeatNum: &seat, CreatedAt: base, UpdatedAt: base,
	})

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(),
		ClientUpdatedAt: base.Add(time.Minute),
		Payload:         &SyncPayload{Notes: syncStr("no gift")},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("notes-only update should apply, got %v", res[0])
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.Name != "Full" || g.Pax != 2 || g.Email != "a@example.com" ||
		g.AngbaoAmt == nil || *g.AngbaoAmt != 888 ||
		g.GiftItem == nil || *g.GiftItem != "Teapot" ||
		g.TableID == nil || *g.TableID != tableID ||
		g.SeatNum == nil || *g.SeatNum != 4 ||
		g.Notes != "no gift" {
		t.Fatalf("partial update clobbered fields: %+v", g)
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
		Payload:         &SyncPayload{Name: syncStr("Skewy-new"), Pax: syncInt(1)},
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
		Payload:         &SyncPayload{Name: syncStr("Collision"), Pax: syncInt(1)},
	}})
	testutil.NoErr(t, err)
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
	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: syncStr("ValidSeat"), Pax: syncInt(1), TableID: syncTable(tID), SeatNum: &seat},
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
		Payload: &SyncPayload{Name: syncStr("GhostTable"), Pax: syncInt(1), TableID: syncTable(missingID), SeatNum: &seat},
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
		Payload: &SyncPayload{Name: syncStr("TooDeep"), Pax: syncInt(1), TableID: syncTable(tID), SeatNum: &bigSeat},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("seat over capacity should fail, got %v", res[0])
	}

	// Create with unknown table -> failed too
	res, err = svc.Sync(ctx, wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: base.Add(time.Minute),
		Payload: &SyncPayload{Name: syncStr("FreshGhost"), Pax: syncInt(1), TableID: syncTable(missingID), SeatNum: &seat},
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
	count, err := svc.BulkCreate(ctx, batch, nil)
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
	count, err = svc.BulkCreate(ctx, clean, nil)
	if err != nil || count != 2 {
		t.Fatalf("clean batch should create 2, got %d err %v", count, err)
	}
}

// Sync must apply the same RSVP whitelist as the HTTP handlers.
func TestSync_RejectsUnknownRSVP(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	name, rsvp := "Bad RSVP", "yes"
	m := SyncMutation{Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: time.Now(), Payload: &SyncPayload{Name: &name, RSVP: &rsvp}}
	res, err := svc.Sync(context.Background(), wid, []SyncMutation{m})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("unknown RSVP must fail, got %s", res[0].Status)
	}
}

// A phone that retries a checkin after a lost response must not duplicate
// the note it carried.
func TestSync_CheckInReplayDoesNotDuplicateNotes(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour).UTC()
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Retry", Pax: 1, Notes: "vip", CreatedAt: base, UpdatedAt: base})

	note := "brought gift"
	m := SyncMutation{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute), Payload: &SyncPayload{Notes: &note}}
	for range 2 {
		res, err := svc.Sync(ctx, wid, []SyncMutation{m})
		testutil.NoErr(t, err)
		if res[0].Status != "applied" {
			t.Fatalf("checkin status = %s (%s)", res[0].Status, res[0].Reason)
		}
	}
	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.Notes != "vip\nbrought gift" {
		t.Fatalf("notes = %q, want one appended line", g.Notes)
	}
}

// A lookup error that is not "no such row" must report failed, so the client
// keeps the mutation queued instead of dropping an offline check-in.
func TestSync_LookupErrorIsFailedNotSkipped(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	testutil.NoErr(t, db.Exec("DROP TABLE guest_records").Error)
	for _, op := range []SyncOp{SyncOpUpdate, SyncOpCheckIn, SyncOpCheckOut, SyncOpDelete} {
		t.Run(string(op), func(t *testing.T) {
			res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
				Op: op, GuestID: uuid.New().String(), ClientUpdatedAt: time.Now(),
			}})
			testutil.NoErr(t, err)
			if res[0].Status != "failed" {
				t.Fatalf("status = %q (%s), want failed", res[0].Status, res[0].Reason)
			}
			if !res[0].Retryable {
				t.Fatal("a database error must be retryable so the client keeps the edit")
			}
		})
	}
}

// An offline edit that clears the angbao or gift sends JSON null; the server
// must clear the value, as the online PUT does, not keep the old one.
func TestSyncUpdate_NullClearsAngbaoAndGift(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	gid := uuid.New()
	amt, gift := 200, "Bangle"
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "A", Pax: 1, AngbaoAmt: &amt, GiftItem: &gift,
		UpdatedAt: time.Now().Add(-time.Hour),
	}).Error)

	var m SyncMutation
	testutil.NoErr(t, json.Unmarshal([]byte(`{"op":"update","guestId":"`+gid.String()+
		`","clientUpdatedAt":"`+time.Now().Format(time.RFC3339Nano)+
		`","payload":{"name":"A","angbaoAmt":null,"giftItem":null}}`), &m))
	res, err := svc.Sync(t.Context(), wid, []SyncMutation{m})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("status = %q (%s), want applied", res[0].Status, res[0].Reason)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.AngbaoAmt != nil || got.GiftItem != nil {
		t.Fatalf("angbao=%v gift=%v, want both cleared", got.AngbaoAmt, got.GiftItem)
	}
}

// A queued check-in for a guest someone already checked in must keep the
// first arrival time and the angbao already keyed, as the online path does.
func TestSyncCheckIn_AlreadyCheckedInKeepsAngbaoAndArrival(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	gid := uuid.New()
	arrived := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	amt := 200
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "A", Pax: 1, CheckedInAt: &arrived, AngbaoAmt: &amt,
		UpdatedAt: arrived,
	}).Error)

	other := 100
	res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
		Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: time.Now(),
		Payload: &SyncPayload{AngbaoAmt: NullableJSON[int]{Set: true, Value: &other}, Notes: syncStr("came late")},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("status = %q (%s), want applied", res[0].Status, res[0].Reason)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.AngbaoAmt == nil || *got.AngbaoAmt != 200 {
		t.Fatalf("angbao = %v, want the first-keyed 200", got.AngbaoAmt)
	}
	if got.CheckedInAt == nil || !got.CheckedInAt.Equal(arrived) {
		t.Fatalf("checked_in_at = %v, want first arrival %v", got.CheckedInAt, arrived)
	}
	if got.Notes != "came late" {
		t.Fatalf("notes = %q, want the queued note appended", got.Notes)
	}
}

// An offline edit made on a copy older than the server's must not apply: here
// it would clear the angbao another desk keyed after the copy was loaded.
func TestSync_StaleBaseKeepsOtherDesksAngbao(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	gid := uuid.New()
	loaded := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	keyed := loaded.Add(15 * time.Minute)
	amt := 288
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Tan", Pax: 1, AngbaoAmt: &amt, UpdatedAt: keyed}).Error)

	res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: keyed.Add(5 * time.Minute), BaseUpdatedAt: &loaded,
		Payload: &SyncPayload{Name: syncStr("Tan Ah Kow"), AngbaoAmt: NullableJSON[int]{Set: true}},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" || res[0].Retryable {
		t.Fatalf("status = %q retryable=%v, want a non-retryable failure", res[0].Status, res[0].Retryable)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.AngbaoAmt == nil || *got.AngbaoAmt != amt || got.Name != "Tan" {
		t.Fatalf("guest = %q angbao %v, want the server copy untouched", got.Name, got.AngbaoAmt)
	}
}

// With no client base, a write that lands between this sync's read and its own
// write is caught too, and reported retryable so the edit stays queued.
func TestSyncUpdate_ChangeAfterLoadIsRetryable(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	gid := uuid.New()
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Lim", Pax: 1, UpdatedAt: at}).Error)
	g, err := svc.Get(t.Context(), gid, wid)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Update("updated_at", at.Add(time.Minute)).Error)

	m := SyncMutation{Op: SyncOpUpdate, GuestID: gid.String()}
	err = svc.syncUpdate(t.Context(), g, m, time.Now())
	if !errors.Is(err, errGuestChangedDuringSync) {
		t.Fatalf("err = %v, want errGuestChangedDuringSync", err)
	}
	if !syncFailed(m, err).Retryable {
		t.Fatal("a change during sync must stay retryable")
	}
}

// A device whose clock runs ahead gets its opTime clamped to server time, so
// a replay of its applied edit carries a different opTime than the row. The
// replay must still report applied, not a stale-base failure.
func TestSync_ReplayFromFastClockIsApplied(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, CreatedAt: t1, UpdatedAt: t1}).Error)

	batch := []SyncMutation{{
		Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: time.Now().Add(time.Hour), BaseUpdatedAt: &t1,
		Payload: &SyncPayload{Name: syncStr("Y")},
	}}
	for round := range 2 {
		res, err := svc.Sync(t.Context(), wid, batch)
		testutil.NoErr(t, err)
		if res[0].Status != "applied" {
			t.Fatalf("round %d: %s (%s), want applied", round, res[0].Status, res[0].Reason)
		}
	}
}

// A checkout replayed after its response was lost (or sent by a second
// device) finds the guest already checked out: intent holds, so "applied",
// not a non-retryable failure that drops it after N attempts.
func TestSync_CheckOutReplayIsApplied(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, CheckedInAt: &t1, CreatedAt: t1, UpdatedAt: t1}).Error)

	batch := []SyncMutation{{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute)}}
	for round := range 2 {
		res, err := svc.Sync(ctx, wid, batch)
		testutil.NoErr(t, err)
		if res[0].Status != "applied" {
			t.Fatalf("round %d: %s (%s), want applied", round, res[0].Status, res[0].Reason)
		}
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt != nil {
		t.Fatal("guest still checked in")
	}
}

// Offline undo of a mistaken check-in survives an unrelated newer edit (a
// notes change) but still loses to a check-in newer than it, and never moves
// updated_at backwards.
func TestSync_CheckOutBeatsUnrelatedNewerEdit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	edited := t1.Add(3 * time.Minute)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, Notes: "desk B", CheckedInAt: &t1, CreatedAt: t1, UpdatedAt: edited}).Error)

	res, err := svc.Sync(ctx, wid, []SyncMutation{{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute)}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("status = %s (%s), want applied", res[0].Status, res[0].Reason)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt != nil {
		t.Fatal("guest still checked in")
	}
	if !got.UpdatedAt.After(edited) {
		t.Fatalf("updated_at = %v, want after %v", got.UpdatedAt, edited)
	}

	// A check-in newer than the queued undo wins.
	later := t1.Add(10 * time.Minute)
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Updates(map[string]any{"checked_in_at": later, "updated_at": later}).Error)
	res, err = svc.Sync(ctx, wid, []SyncMutation{{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t1.Add(5 * time.Minute)}})
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("status = %s, want skipped behind newer check-in", res[0].Status)
	}
}

// Offline edits follow the online PUT rules: a blank name or out-of-range
// pax fails (not retryable) instead of being dropped and reported applied,
// and an empty RSVP clears it as online does.
func TestSync_UpdateValidatesLikeOnline(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 2, RSVP: "confirmed", CreatedAt: t1, UpdatedAt: t1}).Error)

	for name, p := range map[string]*SyncPayload{
		"blank name": {Name: syncStr("  ")},
		"zero pax":   {Pax: syncInt(0)},
	} {
		res, err := svc.Sync(ctx, wid, []SyncMutation{{Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute), Payload: p}})
		testutil.NoErr(t, err)
		if res[0].Status != "failed" || res[0].Retryable {
			t.Fatalf("%s: got %s retryable=%v (%s), want non-retryable failed", name, res[0].Status, res[0].Retryable, res[0].Reason)
		}
	}
	res, err := svc.Sync(ctx, wid, []SyncMutation{{Op: SyncOpUpdate, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute), Payload: &SyncPayload{RSVP: syncStr("")}}})
	testutil.NoErr(t, err)
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if res[0].Status != "applied" || got.RSVP != "" {
		t.Fatalf("clear RSVP: got %s, rsvp %q; want applied and cleared", res[0].Status, got.RSVP)
	}
}

// A mutation without clientUpdatedAt would stamp a check-in as year 1.
func TestSync_ZeroClientTimeFails(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, CreatedAt: t1, UpdatedAt: t1}).Error)
	res, err := svc.Sync(context.Background(), wid, []SyncMutation{{Op: SyncOpCheckIn, GuestID: gid.String()}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" || res[0].Retryable {
		t.Fatalf("got %s retryable=%v, want non-retryable failed", res[0].Status, res[0].Retryable)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt != nil {
		t.Fatalf("checked in at %v", got.CheckedInAt)
	}
}

// A create replayed after the guest was deleted (its first response was
// lost) is skipped, not applied again.
func TestSync_ReplayedCreateDoesNotResurrectDeletedGuest(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	create := []SyncMutation{{Op: SyncOpCreate, GuestID: gid.String(), ClientUpdatedAt: time.Now().Add(-time.Minute), Payload: &SyncPayload{Name: new("Walk-in")}}}
	res, err := svc.Sync(ctx, wid, create)
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("create: %s (%s)", res[0].Status, res[0].Reason)
	}
	testutil.NoErr(t, svc.Delete(ctx, gid, wid))

	res, err = svc.Sync(ctx, wid, create)
	testutil.NoErr(t, err)
	if res[0].Status != "skipped" {
		t.Fatalf("replayed create: %s (%s), want skipped", res[0].Status, res[0].Reason)
	}
	var n int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Count(&n).Error)
	if n != 0 {
		t.Fatal("deleted guest was recreated")
	}
}

// An op queued after a create that failed retryably stays queued with it,
// instead of finding no row and being dropped.
func TestSync_OpAfterRetryableCreateFailureStaysQueued(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	// Break only the create path: guest lookups still work.
	testutil.NoErr(t, db.Migrator().DropTable(&models.GuestTombstone{}))
	gid := uuid.New()
	at := time.Now().Add(-time.Minute)
	res, err := svc.Sync(ctx, wid, []SyncMutation{
		{Op: SyncOpCreate, GuestID: gid.String(), ClientUpdatedAt: at, Payload: &SyncPayload{Name: new("Walk-in")}},
		{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: at.Add(time.Second)},
	})
	testutil.NoErr(t, err)
	for i, r := range res {
		if r.Status != "failed" || !r.Retryable {
			t.Errorf("result %d: %s retryable=%v (%s), want retryable failure", i, r.Status, r.Retryable, r.Reason)
		}
	}
}

// An offline check-in whose note would push notes past MaxGuestNotesLen
// still applies, with the note cut to fit, instead of failing for good.
func TestSync_CheckInNoteCutToFitNotesLimit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	t1 := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	gid := uuid.New()
	existing := strings.Repeat("n", MaxGuestNotesLen-10)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "X", Pax: 1, Notes: existing, CreatedAt: t1, UpdatedAt: t1}).Error)

	angbao := 88
	res, err := svc.Sync(ctx, wid, []SyncMutation{{Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: t1.Add(time.Minute),
		Payload: &SyncPayload{Notes: new("arrived with two extra guests"), AngbaoAmt: NullableJSON[int]{Set: true, Value: &angbao}}}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("status = %s (%s), want applied", res[0].Status, res[0].Reason)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt == nil || got.AngbaoAmt == nil || *got.AngbaoAmt != angbao {
		t.Fatalf("check-in or angbao lost: checkedInAt=%v angbao=%v", got.CheckedInAt, got.AngbaoAmt)
	}
	if want := existing + "\narrived w"; got.Notes != want {
		t.Fatalf("notes = ...%q, want ...%q", got.Notes[len(existing):], want[len(existing):])
	}
}
