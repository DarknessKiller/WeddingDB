package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
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
	err := svc.UpdateWithSeat(context.Background(), g, &tid, 2)
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
	if err := svc.UpdateWithSeat(context.Background(), g, &tid, 4); err != nil {
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
