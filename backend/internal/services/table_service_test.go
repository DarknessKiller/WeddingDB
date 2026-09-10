package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
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

	client := hub.Subscribe(wid)

	svc := NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), hub)
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
