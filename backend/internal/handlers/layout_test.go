package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

func TestLayoutSave_Version(t *testing.T) {
	db := newGuestTestDB(t)
	wid, tid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO wedding_events (id, name) VALUES (?, 'W')`, wid).Error)
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error)
	seat := 5
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Ann", Pax: 1, TableID: &tid, SeatNum: &seat}).Error)

	guestRepo, tableRepo := repository.NewGuestRepo(db), repository.NewTableRepo(db)
	h := NewLayoutHandler(services.NewLayoutService(repository.NewLayoutRepo(db)),
		services.NewTableService(tableRepo, guestRepo, nil, db),
		services.NewWeddingService(repository.NewWeddingRepo(db)))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/weddings/{wid}/layout", h.Get)
	fuego.Patch(s, "/api/weddings/{wid}/layout", h.Save)
	path := "/api/weddings/" + wid.String() + "/layout"
	table := `"tables":[{"id":"` + tid.String() + `","x":1,"y":1,"capacity":10}]`

	version := func() int {
		rec := serveJSON(s, http.MethodGet, path, "")
		var got struct{ Version int }
		testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return got.Version
	}
	if v := version(); v != 0 {
		t.Fatalf("initial version = %d, want 0", v)
	}

	tests := []struct {
		name, body string
		want       int
		wantVer    int
	}{
		{"current version saves", `{"version":0,"hallWidth":900,"hallHeight":900,` + table + `}`, http.StatusOK, 1},
		{"stale version conflicts", `{"version":0,"hallWidth":900,"hallHeight":900,` + table + `}`, http.StatusConflict, 1},
		{"no version saves", `{"hallWidth":900,"hallHeight":900,` + table + `}`, http.StatusOK, 2},
		{"capacity below seated", `{"version":2,"hallWidth":900,"hallHeight":900,"tables":[{"id":"` + tid.String() + `","x":1,"y":1,"capacity":4}]}`, http.StatusBadRequest, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveJSON(s, http.MethodPatch, path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusOK {
				var got struct{ Version int }
				testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &got))
				if got.Version != tt.wantVer {
					t.Fatalf("response version = %d, want %d", got.Version, tt.wantVer)
				}
			}
			if v := version(); v != tt.wantVer {
				t.Fatalf("stored version = %d, want %d", v, tt.wantVer)
			}
		})
	}
}
