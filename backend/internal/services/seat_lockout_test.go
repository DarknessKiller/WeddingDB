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
)

// Shrinking a table below a seated guest must be refused, so the guest is not
// stranded past the new capacity.
func TestTableUpdate_RefusesShrinkBelowSeatedGuest(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 12)
	g := seedGuest(t, svc, wid, "Seated")
	g.TableID, g.SeatNum, g.Pax = &tid, new(10), 1
	testutil.NoErr(t, db.Save(g).Error)

	tables := NewTableService(repository.NewTableRepo(db), nil, db)
	update := &models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 8}
	if err := tables.Update(context.Background(), update); !errors.Is(err, repository.ErrCapacityBelowSeated) {
		t.Fatalf("shrink below seated guest err = %v, want ErrCapacityBelowSeated", err)
	}
	var stored models.BanquetTable
	testutil.NoErr(t, db.First(&stored, "id = ?", tid).Error)
	if stored.Capacity != 12 {
		t.Fatalf("capacity = %d, want the refused write to leave 12", stored.Capacity)
	}

	update.Capacity = 10
	testutil.NoErr(t, tables.Update(context.Background(), update))
}

// A guest already seated past the current capacity (from a legacy row) must
// still be able to edit every field that is not the seat or the party size.
func TestGuestUpdate_SeatRulesRunOnlyForSeatAndPax(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	tid := seedTable(t, svc, wid, "T1", 8)
	g := seedGuest(t, svc, wid, "Past")
	g.TableID, g.SeatNum, g.Pax = &tid, new(10), 1
	testutil.NoErr(t, db.Save(g).Error)

	var notes GuestPatch
	testutil.NoErr(t, json.Unmarshal([]byte(`{"notes":"late arrival","rsvp":"confirmed"}`), &notes))
	if _, err := svc.UpdatePatch(context.Background(), g.ID, wid, &notes); err != nil {
		t.Fatalf("notes edit on a seat past capacity: %v", err)
	}

	var pax GuestPatch
	testutil.NoErr(t, json.Unmarshal([]byte(`{"pax":2}`), &pax))
	if _, err := svc.UpdatePatch(context.Background(), g.ID, wid, &pax); !errors.Is(err, ErrSeatRange) {
		t.Fatalf("pax edit that outgrows the table err = %v, want ErrSeatRange", err)
	}
}

// The offline check-in path writes the angbao amount directly, so it must apply
// the same non-negative rule as the online edit, and must not be retried.
func TestSync_CheckInRejectsNegativeAngbao(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	g := seedGuest(t, svc, wid, "Angbao")

	res, err := svc.Sync(context.Background(), wid, []SyncMutation{{
		Op: SyncOpCheckIn, GuestID: g.ID.String(), ClientUpdatedAt: time.Now(),
		Payload: &SyncPayload{AngbaoAmt: new(-5)},
	}})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res[0].Status != "failed" || res[0].Retryable == nil || *res[0].Retryable {
		t.Fatalf("negative angbao check-in = %+v, want failed with retryable false", res[0])
	}
}

// A validation failure fails without retry: re-sending the same mutation can
// only produce the same refusal (Offline sync invariant 7).
func TestSync_ValidationFailureIsNotRetryable(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	g := seedGuest(t, svc, wid, "Long name")

	res, err := svc.Sync(context.Background(), wid, []SyncMutation{{
		Op: SyncOpUpdate, GuestID: g.ID.String(), ClientUpdatedAt: time.Now(),
		Payload: &SyncPayload{Name: strings.Repeat("x", MaxNameLen+1)},
	}})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res[0].Status != "failed" || res[0].Retryable == nil || *res[0].Retryable {
		t.Fatalf("validation failure = %+v, want failed with retryable false", res[0])
	}
}
