package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublicGuestJSONOmitsPrivateFields(t *testing.T) {
	data, err := json.Marshal(publicGuest{
		ID:   "guest-id",
		Name: "Alice",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"phone", "email", "checkedInAt", "rsvp"} {
		if _, present := fields[field]; present {
			t.Errorf("public guest JSON contains %q: %s", field, data)
		}
	}
}

// TestPublicGuestJSONExposesKioskFields covers fix 8: the kiosk UI and
// frontend/api/public.ts expect isVip and checkedInAt on the public payload.
func TestPublicGuestJSONExposesKioskFields(t *testing.T) {
	data, err := json.Marshal(publicGuest{
		ID:    "guest-id",
		Name:  "Alice",
		IsVip: true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if vip, ok := fields["isVip"].(bool); !ok || !vip {
		t.Errorf("public guest JSON should expose isVip=true: %s", data)
	}
}

// A byte-slice cap splits multibyte runes and hands Postgres invalid UTF-8.
func TestLimitQuery_KeepsValidUTF8(t *testing.T) {
	got := limitQuery(strings.Repeat("婚", 150), 100)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated query is invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 100 {
		t.Fatalf("rune count = %d, want 100", n)
	}
	if short := limitQuery("abc", 100); short != "abc" {
		t.Fatalf("short query changed to %q", short)
	}
}

// The public list and search must not leak seat numbers when the wedding
// hides them from the kiosk. Requires Postgres (search uses ILIKE).
func TestPublicGuests_HideSeatNumbers(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() { db.Exec("DROP TABLE IF EXISTS guest_records, wedding_events CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.AutoMigrate(&models.WeddingEvent{}, &models.GuestRecord{}))

	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{WeddingID: wid, Name: "Alice", SeatNum: new(3)}).Error)

	h := NewPublicGuestHandler(
		services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db),
		services.NewWeddingService(repository.NewWeddingRepo(db)),
	)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/w/{wid}/guests", h.List)
	fuego.Get(s, "/w/{wid}/search", h.Search)
	seats := func(path string) []any {
		t.Helper()
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/w/"+utils.EncodeUUID(wid)+path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, body %s", path, rec.Code, rec.Body.String())
		}
		var list []map[string]any
		if path == "/guests" {
			var body struct{ Guests []map[string]any }
			testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &body))
			list = body.Guests
		} else {
			testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &list))
		}
		out := make([]any, 0, len(list))
		for _, g := range list {
			out = append(out, g["seatNum"])
		}
		return out
	}

	for _, path := range []string{"/guests", "/search?q=Ali"} {
		if got := seats(path); len(got) != 1 || got[0] != nil {
			t.Errorf("%s with seats hidden: seatNum = %v, want [nil]", path, got)
		}
	}
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Where("id = ?", wid).Update("show_seat_numbers", true).Error)
	for _, path := range []string{"/guests", "/search?q=Ali"} {
		if got := seats(path); len(got) != 1 || got[0] != float64(3) {
			t.Errorf("%s with seats shown: seatNum = %v, want [3]", path, got)
		}
	}
}

// The kiosk link exposes guest names; it must stop answering once the
// wedding is over instead of staying public forever.
func TestKioskOpen_ClosesAfterTheWedding(t *testing.T) {
	day := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	w := &models.WeddingEvent{Date: day}
	for _, tc := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{"any time before the date", day.Add(-200 * 24 * time.Hour), true},
		{"week of grace", day.Add(5 * 24 * time.Hour), true},
		{"after the grace week", day.Add(8 * 24 * time.Hour), false},
	} {
		if got := kioskOpen(w, tc.now); got != tc.want {
			t.Errorf("%s: kioskOpen = %v, want %v", tc.name, got, tc.want)
		}
	}
}
