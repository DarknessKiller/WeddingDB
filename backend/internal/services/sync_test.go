package services

import (
	"context"
	"encoding/json"
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
	if !g.UpdatedAt.Equal(base.Add(5 * time.Minute)) {
		t.Fatalf("updated_at moved backwards: %v", g.UpdatedAt)
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
