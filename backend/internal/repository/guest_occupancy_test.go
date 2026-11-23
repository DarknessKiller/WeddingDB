package repository

import (
	"encoding/json"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTableOccupancy_HighestSeat covers the empty table (0) and scattered
// seats (highest seat + party size), keeping the Pax sum and the JSON names.
func TestTableOccupancy_HighestSeat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		capacity INTEGER, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
		degree REAL NOT NULL DEFAULT 0, is_vip INTEGER,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1,
		table_id TEXT, seat_num INTEGER, rsvp TEXT DEFAULT 'no_response',
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER,
		angbao_amt INTEGER, gift_item TEXT, field_times TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error)

	ctx := t.Context()
	wid := uuid.New()
	empty, scattered := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: empty, WeddingID: wid, Name: "Empty", Capacity: 8}).Error)
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: scattered, WeddingID: wid, Name: "Scattered", Capacity: 8}).Error)

	seat := func(n int) *int { return &n }
	table := func(id uuid.UUID) *uuid.UUID { return &id }
	guests := []models.GuestRecord{
		{WeddingID: wid, Name: "Seat 1", Pax: 2, TableID: table(scattered), SeatNum: seat(1)},
		{WeddingID: wid, Name: "Seat 4", Pax: 3, TableID: table(scattered), SeatNum: seat(4)},
		{WeddingID: wid, Name: "Seat 5", Pax: 1, TableID: table(scattered), SeatNum: seat(5)},
		// Unseated but assigned: counts toward Pax, never toward HighestSeat.
		{WeddingID: wid, Name: "No seat", Pax: 4, TableID: table(scattered)},
	}
	for i := range guests {
		testutil.NoErr(t, db.Create(&guests[i]).Error)
	}

	occ, err := NewGuestRepo(db).TableOccupancy(ctx, wid)
	testutil.NoErr(t, err)
	if len(occ) != 2 {
		t.Fatalf("occupancy rows = %d, want 2", len(occ))
	}
	byTable := make(map[uuid.UUID]TableOccupancy, len(occ))
	for _, o := range occ {
		byTable[o.TableID] = o
	}
	if got := byTable[empty]; got.Pax != 0 || got.HighestSeat != 0 {
		t.Fatalf("empty table = %+v, want pax 0 highest seat 0", got)
	}
	if got := byTable[scattered]; got.Pax != 10 || got.HighestSeat != 6 {
		t.Fatalf("scattered table = %+v, want pax 10 highest seat 6", got)
	}

	body, err := json.Marshal(byTable[scattered])
	testutil.NoErr(t, err)
	var decoded map[string]any
	testutil.NoErr(t, json.Unmarshal(body, &decoded))
	if decoded["HighestSeat"] != float64(6) || decoded["Pax"] != float64(10) {
		t.Fatalf("json = %s, want HighestSeat 6 and Pax 10", body)
	}
}
