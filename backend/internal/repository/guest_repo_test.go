package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"weddingdb/internal/models"
)

type searchSQLLogger struct {
	logger.Interface
	sql string
}

func (l *searchSQLLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	l.sql, _ = fc()
}

func newMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	// :memory: is per-connection; keep the pool at one connection.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	// Raw DDL: AutoMigrate chokes on `default:gen_random_uuid()` in sqlite.
	if err := db.Exec(`CREATE TABLE guest_records (
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
		walk_in INTEGER DEFAULT 0,
		notes TEXT,
		dietary TEXT,
		is_vip INTEGER,
		angbao_amt INTEGER,
		gift_item TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create guest_records: %v", err)
	}
	return db
}

// A guest checked in while not confirmed is a walk-in: RSVP is promoted to
// confirmed (so the check-in rate denominator counts them) and walk_in is
// sticky across checkout + re-check-in.
func TestConditionalCheckInPromotesWalkIn(t *testing.T) {
	db := newMemoryDB(t)
	repo := NewGuestRepo(db)
	ctx := context.Background()
	wid := uuid.New()
	now := time.Now()

	pending := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Pending", Pax: 2, RSVP: "pending"}
	confirmed := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Confirmed", Pax: 1, RSVP: "confirmed"}
	for _, g := range []*models.GuestRecord{pending, confirmed} {
		if err := db.Create(g).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	if err := repo.ConditionalCheckIn(ctx, pending.ID, wid, now); err != nil {
		t.Fatalf("check in pending: %v", err)
	}
	reload := func(id uuid.UUID) models.GuestRecord {
		t.Helper()
		var g models.GuestRecord
		if err := db.First(&g, "id = ?", id).Error; err != nil {
			t.Fatalf("reload %s: %v", id, err)
		}
		return g
	}
	got := reload(pending.ID)
	if got.RSVP != "confirmed" || !got.WalkIn || got.CheckedInAt == nil {
		t.Errorf("pending check-in: rsvp=%q walkIn=%v checkedInAt=%v; want confirmed/true/set", got.RSVP, got.WalkIn, got.CheckedInAt)
	}

	if err := repo.ConditionalCheckIn(ctx, pending.ID, wid, now); !errors.Is(err, ErrAlreadyCheckedIn) {
		t.Errorf("second check-in err = %v, want ErrAlreadyCheckedIn", err)
	}

	// checkout + re-check-in must not clear walk_in
	db.Model(&models.GuestRecord{}).Where("id = ?", pending.ID).Update("checked_in_at", nil)
	if err := repo.ConditionalCheckIn(ctx, pending.ID, wid, now); err != nil {
		t.Fatalf("re-check-in: %v", err)
	}
	if !reload(pending.ID).WalkIn {
		t.Error("walk_in cleared by re-check-in")
	}

	if err := repo.ConditionalCheckIn(ctx, confirmed.ID, wid, now); err != nil {
		t.Fatalf("check in confirmed: %v", err)
	}
	if reload(confirmed.ID).WalkIn {
		t.Error("confirmed guest marked walk-in")
	}
}

func TestSearchByWeddingMatchesNamesOnly(t *testing.T) {
	capture := &searchSQLLogger{Interface: logger.Default}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: capture,
	})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}

	repo := NewGuestRepo(db)
	for _, query := range []string{"张", "13800138000", "guest@example.com"} {
		t.Run(query, func(t *testing.T) {
			capture.sql = ""
			if _, err := repo.SearchByWedding(context.Background(), uuid.New(), query); err != nil {
				t.Fatalf("search: %v", err)
			}
			if !strings.Contains(capture.sql, "name ILIKE") || !strings.Contains(capture.sql, "name_pinyin ILIKE") {
				t.Errorf("SQL must match name and pinyin: %q", capture.sql)
			}
			lowerSQL := strings.ToLower(capture.sql)
			if strings.Contains(lowerSQL, "phone") || strings.Contains(lowerSQL, "email") {
				t.Errorf("SQL must not match contact fields: %q", capture.sql)
			}
		})
	}
}
