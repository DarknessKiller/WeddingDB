package services

import (
	"context"
	"errors"
	"slices"
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

func tableIDPtr(id uuid.UUID) *string {
	s := id.String()
	return &s
}

// Offline sync must apply the same overlap rule as AssignSeat, not just a
// capacity check.
func TestSyncCreate_RejectsSeatOverlap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	now := time.Now()

	first := SyncMutation{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: now,
		Payload: &SyncPayload{Name: "A", Pax: 1, TableID: tableIDPtr(tid), SeatNum: intPtr(3)},
	}
	firstRes, err := svc.Sync(context.Background(), wid, []SyncMutation{first})
	if err != nil || firstRes[0].Status != "applied" {
		t.Fatalf("first seat should apply: %+v %v", firstRes[0], err)
	}

	second := SyncMutation{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: now.Add(time.Second),
		Payload: &SyncPayload{Name: "B", Pax: 1, TableID: tableIDPtr(tid), SeatNum: intPtr(3)},
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

// An import reads each table's occupants once, not once per seated row, and
// still sees the rows it inserted earlier in the batch. The other guest read is
// the one lookup of already-imported IDs.
func TestBulkCreate_ReadsTableOccupancyOnce(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	guestReads := 0
	testutil.NoErr(t, svc.db.Callback().Query().After("gorm:query").Register("count_guest_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "guest_records" {
			guestReads++
		}
	}))

	batch := make([]models.GuestRecord, 0, 5)
	for i := range 5 {
		batch = append(batch, models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "G", Pax: 1, TableID: &tid, SeatNum: intPtr(i + 1)})
	}
	created, err := svc.BulkCreate(context.Background(), batch)
	testutil.NoErr(t, err)
	if created != 5 || guestReads != 2 {
		t.Fatalf("created = %d, guest reads = %d, want 5 and 2", created, guestReads)
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

// renameAndSeat is one online edit that renames the guest and moves the seat.
func renameAndSeat(tid uuid.UUID, seat int) *GuestPatch {
	table := tid.String()
	return &GuestPatch{Name: "Renamed", TableID: &table, SeatNum: &seat, set: map[string]bool{"name": true, "tableId": true}}
}

// UpdatePatch is one transaction: a failed seat check must leave the edited
// fields untouched.
func TestUpdatePatch_FailedSeatLeavesFieldsUntouched(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	g := seedGuest(t, svc, wid, "Alice")
	other := seedGuest(t, svc, wid, "Bob")
	if err := svc.AssignSeat(context.Background(), other.ID, wid, tid, 2); err != nil {
		t.Fatalf("assign seat: %v", err)
	}

	_, err := svc.UpdatePatch(context.Background(), g.ID, wid, renameAndSeat(tid, 2))
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
func TestUpdatePatch_AppliesNameAndSeat(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	g := seedGuest(t, svc, wid, "Alice")

	if _, err := svc.UpdatePatch(context.Background(), g.ID, wid, renameAndSeat(tid, 4)); err != nil {
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

// A multi-seat run occupies every seat up to seatNum+pax-1; a newcomer that
// overlaps only the tail of that run must still be refused.
func TestAssignSeat_RejectsMultiSeatRunOverlap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	host := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Host", Pax: 3, TableID: &tid, SeatNum: intPtr(2)}
	if err := svc.db.Create(host).Error; err != nil {
		t.Fatalf("seed host: %v", err)
	}
	newcomer := seedGuest(t, svc, wid, "Newcomer")

	// Seats 2-4 are taken; seat 4 is the tail.
	if err := svc.AssignSeat(context.Background(), newcomer.ID, wid, tid, 4); !errors.Is(err, ErrSeatOverlap) {
		t.Fatalf("seat 4 err = %v, want ErrSeatOverlap", err)
	}
	if err := svc.AssignSeat(context.Background(), newcomer.ID, wid, tid, 5); err != nil {
		t.Fatalf("seat 5 after the run = %v, want nil", err)
	}
}

// The incoming guest can itself be a run: a party at seat 3 with pax 3 overlaps
// a single guest on seat 4.
func TestAssignSeat_RejectsIncomingRunOverlap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	opponent := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Opponent", Pax: 1, TableID: &tid, SeatNum: intPtr(4)}
	if err := svc.db.Create(opponent).Error; err != nil {
		t.Fatalf("seed opponent: %v", err)
	}
	party := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Party", Pax: 3}
	if err := svc.db.Create(party).Error; err != nil {
		t.Fatalf("seed party: %v", err)
	}

	if _, err := svc.UpdatePatch(context.Background(), party.ID, wid, &GuestPatch{
		TableID: tableIDPtr(tid), SeatNum: intPtr(3),
		set: map[string]bool{"tableId": true, "seatNum": true},
	}); !errors.Is(err, ErrSeatOverlap) {
		t.Fatalf("party at 3-5 err = %v, want ErrSeatOverlap", err)
	}
}

// Guests given a table but no seat still count toward its capacity.
func TestBulkCreate_RejectsUnseatedOverCapacity(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 10)
	batch := make([]models.GuestRecord, 0, 6)
	for range 6 {
		batch = append(batch, models.GuestRecord{WeddingID: wid, Name: "G", Pax: 2, TableID: &tid})
	}
	if _, err := svc.BulkCreate(t.Context(), batch); !errors.Is(err, ErrSeatRange) {
		t.Fatalf("12 people on a 10-seat table: err = %v, want ErrSeatRange", err)
	}
	if _, err := svc.BulkCreate(t.Context(), batch[:5]); err != nil {
		t.Fatalf("10 people on a 10-seat table: %v", err)
	}
	g := seedGuest(t, svc, wid, "Late")
	if _, err := svc.UpdatePatch(t.Context(), g.ID, wid, &GuestPatch{
		TableID: tableIDPtr(tid), set: map[string]bool{"tableId": true},
	}); !errors.Is(err, ErrSeatRange) {
		t.Fatalf("11th person without a seat: err = %v, want ErrSeatRange", err)
	}
}

// Re-sending an import whose response was lost skips the rows already written.
func TestBulkCreate_RetryIsIdempotent(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	batch := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "A", Pax: 1},
		{ID: uuid.New(), WeddingID: wid, Name: "B", Pax: 2},
	}
	retry := slices.Clone(batch)
	created, err := svc.BulkCreate(t.Context(), batch)
	testutil.NoErr(t, err)
	again, err := svc.BulkCreate(t.Context(), retry)
	testutil.NoErr(t, err)
	var count int64
	testutil.NoErr(t, svc.db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&count).Error)
	if created != 2 || again != 0 || count != 2 {
		t.Fatalf("created %d then %d, stored %d; want 2, 0, 2", created, again, count)
	}
}

// A large import sends one resync instead of one event per row, which would
// overflow each client's 64-event buffer.
func TestBulkCreate_LargeBatchResyncs(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	svc.sseHub = testSSEHub(t)
	client := testSubscribe(t, svc.sseHub, wid)
	t.Cleanup(func() { svc.sseHub.Unsubscribe(client) })

	batch := make([]models.GuestRecord, bulkResyncThreshold+1)
	for i := range batch {
		batch[i] = models.GuestRecord{WeddingID: wid, Name: "G", Pax: 1}
	}
	_, err := svc.BulkCreate(t.Context(), batch)
	testutil.NoErr(t, err)
	select {
	case <-client.Done:
	case <-time.After(time.Second):
		t.Fatal("client was not asked to resync after a large import")
	}
	if n := len(client.Chan); n != 0 {
		t.Fatalf("client received %d per-row events, want 0", n)
	}
}
