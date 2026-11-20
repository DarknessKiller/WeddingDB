package services

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedTable inserts a banquet table in wid and returns its id.
func seedTable(t *testing.T, svc *GuestService, wid uuid.UUID, name string, capacity int) uuid.UUID {
	t.Helper()
	tid := uuid.New()
	if err := svc.db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: name, Capacity: capacity}).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}
	return tid
}

func seedGuest(t *testing.T, svc *GuestService, wid uuid.UUID, name string) *models.GuestRecord {
	t.Helper()
	g := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: name, Pax: 1}
	if err := svc.db.Create(g).Error; err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	return g
}

// A seat number near the int limit must be rejected instead of wrapping past
// the capacity check.
func TestAssignSeat_RejectsOverflowingSeatRange(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	guest := seedGuest(t, svc, wid, "A")
	guest.Pax = 2
	if err := svc.db.Save(guest).Error; err != nil {
		t.Fatalf("save guest: %v", err)
	}
	if err := svc.AssignSeat(context.Background(), guest.ID, wid, tid, math.MaxInt); err == nil {
		t.Fatal("seat near the int limit should be rejected")
	}
}

// Offline sync must apply the same overlap rule as AssignSeat, not just a
// capacity check.
func TestSyncCreate_RejectsSeatOverlap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	now := time.Now()

	first := SyncMutation{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: now,
		Payload: &SyncPayload{Name: syncStr("A"), Pax: syncInt(1), TableID: syncTable(tid.String()), SeatNum: intPtr(3)},
	}
	firstRes, err := svc.Sync(context.Background(), wid, []SyncMutation{first})
	if err != nil || firstRes[0].Status != "applied" {
		t.Fatalf("first seat should apply: %+v %v", firstRes[0], err)
	}

	second := SyncMutation{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: now.Add(time.Second),
		Payload: &SyncPayload{Name: syncStr("B"), Pax: syncInt(1), TableID: syncTable(tid.String()), SeatNum: intPtr(3)},
	}
	res, err := svc.Sync(context.Background(), wid, []SyncMutation{second})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res[0].Status != "failed" {
		t.Fatalf("overlapping seat must fail, got %+v", res[0])
	}
	var count int64
	if err := svc.db.Model(&models.GuestRecord{}).
		Where("wedding_id = ? AND table_id = ? AND seat_num = ?", wid, tid, 3).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("seat 3 occupants = %d, want 1", count)
	}
}

// A bulk import must not let two rows in the same batch share a seat.
func TestBulkCreate_RejectsSeatOverlap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)

	batch := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "A", Pax: 1, TableID: &tid, SeatNum: intPtr(5)},
		{ID: uuid.New(), WeddingID: wid, Name: "B", Pax: 1, TableID: &tid, SeatNum: intPtr(5)},
	}
	created, err := svc.BulkCreate(context.Background(), batch)
	if !errors.Is(err, ErrSeatOverlap) {
		t.Fatalf("error = %v, want ErrSeatOverlap", err)
	}
	if created != 0 {
		t.Fatalf("created = %d, want 0 (batch rolled back)", created)
	}
}

// A table id from another wedding must not be attachable through an import.
func TestBulkCreate_RejectsForeignTable(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	otherWedding := uuid.New()
	foreign := uuid.New()
	if err := svc.db.Create(&models.BanquetTable{ID: foreign, WeddingID: otherWedding, Name: "X", Capacity: 10}).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}

	batch := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "A", Pax: 1, TableID: &foreign},
	}
	if _, err := svc.BulkCreate(context.Background(), batch); err == nil {
		t.Fatal("foreign table without a seat must be rejected")
	}
}

// UpdateWithSeat is one transaction: a failed seat check must leave the edited
// fields untouched.
func TestUpdateWithSeat_FailedSeatLeavesFieldsUntouched(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	g := seedGuest(t, svc, wid, "Alice")
	other := seedGuest(t, svc, wid, "Bob")
	if err := svc.AssignSeat(context.Background(), other.ID, wid, tid, 2); err != nil {
		t.Fatalf("assign seat: %v", err)
	}

	g.Name = "Renamed"
	err := svc.UpdateWithSeat(context.Background(), g, &tid, 2, nil)
	if !errors.Is(err, ErrSeatOverlap) {
		t.Fatalf("error = %v, want ErrSeatOverlap", err)
	}

	stored, err := repository.NewGuestRepo(svc.db).FindByID(context.Background(), g.ID, wid)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Name != "Alice" {
		t.Fatalf("name = %q, want the unedited value Alice", stored.Name)
	}
	if stored.TableID != nil || stored.SeatNum != nil {
		t.Fatalf("seat changed on failed update: table=%v seat=%v", stored.TableID, stored.SeatNum)
	}
}

// The happy path still moves the guest and saves the fields together.
func TestUpdateWithSeat_AppliesBoth(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	g := seedGuest(t, svc, wid, "Alice")

	g.Name = "Renamed"
	if err := svc.UpdateWithSeat(context.Background(), g, &tid, 4, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	stored, err := repository.NewGuestRepo(svc.db).FindByID(context.Background(), g.ID, wid)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Name != "Renamed" || stored.TableID == nil || *stored.TableID != tid || stored.SeatNum == nil || *stored.SeatNum != 4 {
		t.Fatalf("stored = %+v", stored)
	}
}

// A table without a seat number still enforces capacity: three 4-pax parties
// do not fit on a 10-seat table.
func TestUpdateWithoutSeat_RejectsOverCapacity(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	ctx := context.Background()
	tid := seedTable(t, svc, wid, "T1", 10)
	for i, name := range []string{"A", "B", "C"} {
		g := seedGuest(t, svc, wid, name)
		g.Pax = 4
		g.TableID = &tid
		err := svc.Update(ctx, g)
		if i < 2 && err != nil {
			t.Fatalf("party %s should fit: %v", name, err)
		}
		if i == 2 && err == nil {
			t.Fatal("third 4-pax party must be rejected on a 10-seat table")
		}
	}
}

// Seat moves and check-out write only their own columns, so a concurrent
// check-in or notes edit made after the guest was loaded survives.
func TestSeatAndCheckOut_KeepConcurrentEdits(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	tid := seedTable(t, svc, wid, "T1", 10)
	g := seedGuest(t, svc, wid, "A")

	// Another staff phone writes after service loaded guest but
	// before its own update runs.
	var concurrent func(*gorm.DB)
	testutil.NoErr(t, db.Callback().Update().Before("gorm:update").Register("test:concurrent_write", func(tx *gorm.DB) {
		if f := concurrent; f != nil {
			concurrent = nil
			f(tx.Session(&gorm.Session{NewDB: true}))
		}
	}))

	concurrent = func(tx *gorm.DB) {
		testutil.NoErr(t, tx.Exec("UPDATE guest_records SET checked_in_at = ? WHERE id = ?", time.Now(), g.ID).Error)
	}
	testutil.NoErr(t, svc.AssignSeat(ctx, g.ID, wid, tid, 1))
	got, err := svc.Get(ctx, g.ID, wid)
	testutil.NoErr(t, err)
	if got.CheckedInAt == nil {
		t.Fatal("AssignSeat cleared concurrent check-in")
	}

	concurrent = func(tx *gorm.DB) {
		testutil.NoErr(t, tx.Exec("UPDATE guest_records SET notes = ? WHERE id = ?", "late note", g.ID).Error)
	}
	testutil.NoErr(t, svc.CheckOut(ctx, g.ID, wid))
	got, err = svc.Get(ctx, g.ID, wid)
	testutil.NoErr(t, err)
	if got.CheckedInAt != nil || got.Notes != "late note" || got.TableID == nil {
		t.Fatalf("checkout must clear only checked_in_at: %+v", got)
	}
}

// Seatless guests at a table count toward its capacity on every path: a seat
// assignment and a capacity cut must not ignore them.
func TestCapacity_CountsSeatlessGuests(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	party := seedGuest(t, svc, wid, "Party of 8")
	party.Pax = 8
	party.TableID = &tid
	testutil.NoErr(t, svc.db.Save(party).Error)

	five := seedGuest(t, svc, wid, "Party of 5")
	five.Pax = 5
	testutil.NoErr(t, svc.db.Save(five).Error)
	if err := svc.AssignSeat(t.Context(), five.ID, wid, tid, 1); err == nil {
		t.Fatal("seating 5 at a table of 10 that already holds 8 should fail")
	}

	ts := NewTableService(svc.tableRepo, svc.guestRepo, nil, svc.db)
	table, err := ts.Get(t.Context(), tid, wid)
	testutil.NoErr(t, err)
	table.Capacity = 2
	if err := ts.Update(t.Context(), table); !errors.Is(err, repository.ErrCapacityBelowSeats) {
		t.Fatalf("capacity cut below 8 seatless guests: err = %v, want ErrCapacityBelowSeats", err)
	}
}

// An edit based on a row that someone else has since changed (here, a
// check-in) must be refused instead of overwriting their change.
func TestUpdateWithSeat_RejectsStaleBase(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	g := seedGuest(t, svc, wid, "A")
	loaded, err := svc.Get(t.Context(), g.ID, wid)
	testutil.NoErr(t, err)
	base := loaded.UpdatedAt

	time.Sleep(2 * time.Millisecond) // updated_at must move past base
	testutil.NoErr(t, svc.CheckIn(t.Context(), g.ID, wid, "brought +1"))

	edit := *loaded
	edit.RSVP = "pending"
	if err := svc.UpdateWithSeat(t.Context(), &edit, nil, 0, &base); !errors.Is(err, ErrStaleGuest) {
		t.Fatalf("stale edit: err = %v, want ErrStaleGuest", err)
	}
	fresh, err := svc.Get(t.Context(), g.ID, wid)
	testutil.NoErr(t, err)
	if fresh.Notes != "brought +1" || fresh.RSVP != "confirmed" {
		t.Fatalf("check-in was overwritten: notes=%q rsvp=%q", fresh.Notes, fresh.RSVP)
	}
	fresh.RSVP = "pending"
	freshBase := fresh.UpdatedAt
	testutil.NoErr(t, svc.UpdateWithSeat(t.Context(), fresh, nil, 0, &freshBase))
}

// Missing rows map to not-found sentinels, so handlers answer 404, not 500.
func TestMissingRows_ReturnNotFound(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	if err := svc.CheckOut(t.Context(), uuid.New(), wid); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("check-out of a missing guest: err = %v, want ErrGuestNotFound", err)
	}
	ts := NewTableService(svc.tableRepo, svc.guestRepo, nil, svc.db)
	if err := ts.Delete(t.Context(), uuid.New(), wid); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("delete of a missing table: err = %v, want ErrTableNotFound", err)
	}
}
