package handlers

import (
	"net/http"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A table deleted between the handler's read and its write is a 404, not a
// silent 200 for a write that matched nothing.
func TestTableUpdate_DeletedMidRequestIsNotFound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		capacity INTEGER, x REAL, y REAL, degree REAL, is_vip INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, table_id TEXT,
		seat_num INTEGER, pax INTEGER)`).Error)
	wid, tid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error)

	// Delete the table right after the handler loads it.
	deleted := false
	testutil.NoErr(t, db.Callback().Query().After("gorm:query").Register("test:delete_table", func(tx *gorm.DB) {
		if !deleted && tx.Statement.Table == "banquet_tables" {
			deleted = true
			testutil.NoErr(t, tx.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM banquet_tables").Error)
		}
	}))

	svc := services.NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/tables/{id}", NewTableHandler(svc).Update)
	rec := serveGuest(t, s, http.MethodPut, "/api/weddings/"+wid.String()+"/tables/"+tid.String(), `{"name":"T1","capacity":8}`)
	if !deleted {
		t.Fatal("the table was never deleted mid-request")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
