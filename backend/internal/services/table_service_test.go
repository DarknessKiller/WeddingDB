package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTableDelete_PublishesUnassignedGuests covers fix 7: deleting a table
// must broadcast an "update" event per unassigned guest so other clients do
// not show ghosted seated guests.
func TestTableDelete_PublishesUnassignedGuests(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	hub := NewSSEHub(rdb)
	t.Cleanup(hub.Shutdown)

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

	wid := uuid.New()
	tableID := uuid.New()
	seat := 2
	guest := &models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Seated", Pax: 1,
		TableID: &tableID, SeatNum: &seat,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(guest).Error; err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	if err := db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "T1", Capacity: 8}).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}

	client, err := hub.Subscribe(wid)
	testutil.NoErr(t, err)

	svc := NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), hub, db)
	if err := svc.Delete(context.Background(), tableID, wid); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Guest must be unassigned in the database.
	var got models.GuestRecord
	if err := db.First(&got, "id = ?", guest.ID).Error; err != nil {
		t.Fatalf("reload guest: %v", err)
	}
	if got.TableID != nil || got.SeatNum != nil {
		t.Fatalf("guest should be unassigned, got table=%v seat=%v", got.TableID, got.SeatNum)
	}

	// An SSE "update" event with cleared table/seat must reach subscribers.
	select {
	case raw := <-client.Chan:
		var ev GuestEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatalf("parse event: %v", err)
		}
		if ev.Type != "update" {
			t.Fatalf("event type = %q, want \"update\"", ev.Type)
		}
		if ev.Guest == nil {
			t.Fatal("event payload missing guest")
		}
		if ev.Guest.TableID != nil || ev.Guest.SeatNum != nil {
			t.Fatalf("event guest should have nil table/seat, got table=%v seat=%v", ev.Guest.TableID, ev.Guest.SeatNum)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for unassign SSE event")
	}
}

func TestCapacityCannotDropBelowSeatedGuests(t *testing.T) {
	_, db, wid := newGuestSyncService(t)
	ctx := t.Context()
	tid := uuid.New()
	seat := 3
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error)
	// Seats 3-4.
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Ann", Pax: 2, TableID: &tid, SeatNum: &seat}).Error)
	tables := NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db)
	layouts := repository.NewLayoutRepo(db)

	tests := []struct {
		capacity int
		wantErr  bool
	}{
		{3, true},
		{4, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("table update capacity %d", tt.capacity), func(t *testing.T) {
			err := tables.Update(ctx, &models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: tt.capacity})
			if got := errors.Is(err, repository.ErrCapacityBelowSeated); got != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
		t.Run(fmt.Sprintf("layout save capacity %d", tt.capacity), func(t *testing.T) {
			err := layouts.SaveLayout(ctx, wid, 0, 0, []models.TableUpdate{{ID: tid, Capacity: tt.capacity}}, nil, nil, nil)
			if got := errors.Is(err, repository.ErrCapacityBelowSeated); got != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	var got models.BanquetTable
	testutil.NoErr(t, db.First(&got, "id = ?", tid).Error)
	if got.Capacity != 4 {
		t.Fatalf("capacity = %d, want 4 (rejected writes rolled back)", got.Capacity)
	}
}
