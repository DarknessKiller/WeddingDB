package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

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
		updated_at DATETIME,
		field_updated_at TEXT
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

	client, ok := hub.Subscribe(wid, uuid.New())
	if !ok {
		t.Fatal("Subscribe rejected the first client")
	}

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

// A failed delete must roll back the guest unassign: no half-deleted table
// leaves seated guests detached from a table that still exists.
func TestTableDelete_RollsBackOnFailure(t *testing.T) {
	_, db, wid := newGuestSyncService(t)
	tableID := uuid.New()
	seat := 1
	guest := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Seated", Pax: 1, TableID: &tableID, SeatNum: &seat}
	if err := db.Create(guest).Error; err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	if err := db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "T1", Capacity: 8}).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}
	if err := db.Migrator().DropTable(&models.BanquetTable{}); err != nil {
		t.Fatalf("drop table store: %v", err)
	}

	svc := NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db)
	if err := svc.Delete(context.Background(), tableID, wid); err == nil {
		t.Fatal("delete should fail when the table store is unavailable")
	}

	var got models.GuestRecord
	if err := db.First(&got, "id = ?", guest.ID).Error; err != nil {
		t.Fatalf("reload guest: %v", err)
	}
	if got.TableID == nil || *got.TableID != tableID {
		t.Fatalf("failed delete left the guest unassigned, table = %v", got.TableID)
	}
}

// Shrinking a table below seats already in use must fail on both write paths
// (table form and layout editor) instead of stranding guests past the last seat.
func TestTableCapacity_RejectsCutBelowSeatedGuests(t *testing.T) {
	_, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	tableID := uuid.New()
	seat := 9
	table := &models.BanquetTable{ID: tableID, WeddingID: wid, Name: "T1", Capacity: 10}
	if err := db.Create(table).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}
	if err := db.Create(&models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Pair", Pax: 2, TableID: &tableID, SeatNum: &seat}).Error; err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	svc := NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db)

	table.Capacity = 9
	if err := svc.Update(ctx, table); !errors.Is(err, repository.ErrCapacityBelowSeats) {
		t.Fatalf("table update: want ErrCapacityBelowSeats, got %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT, type TEXT,
		x REAL, y REAL, degree REAL, width REAL, height REAL, color TEXT, text_color TEXT, stroke_color TEXT,
		opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("create hall_elements: %v", err)
	}
	// A capacity the editor sent is checked like the table form's.
	err := repository.NewLayoutRepo(db).SaveLayout(ctx, wid, 0, 0,
		[]models.TableUpdate{{ID: tableID, Capacity: new(9)}}, nil, ReconcileElements)
	if !errors.Is(err, repository.ErrCapacityBelowSeats) {
		t.Fatalf("layout save: want ErrCapacityBelowSeats, got %v", err)
	}
	// A save that leaves capacity out (editor did not change it) must not
	// write back a stale copy: only the position moves.
	if err := repository.NewLayoutRepo(db).SaveLayout(ctx, wid, 0, 0,
		[]models.TableUpdate{{ID: tableID, X: 40, Y: 60}}, nil, ReconcileElements); err != nil {
		t.Fatalf("layout save: %v", err)
	}
	var moved models.BanquetTable
	if err := db.First(&moved, "id = ?", tableID).Error; err != nil {
		t.Fatalf("reload table: %v", err)
	}
	if moved.Capacity != 10 || moved.X != 40 || moved.Y != 60 {
		t.Fatalf("layout save: got capacity %d at (%v,%v), want 10 at (40,60)", moved.Capacity, moved.X, moved.Y)
	}

	table.Capacity = 10
	if err := svc.Update(ctx, table); err != nil {
		t.Fatalf("capacity equal to last seat must pass: %v", err)
	}
}
