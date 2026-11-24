package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// TestUpdateKioskSettings_ResponseCarriesStoredUpdatedAt proves the PUT answers
// the updatedAt the server stored, so a client that caches on it is not told the
// wedding is older than its own write.
func TestUpdateKioskSettings_ResponseCarriesStoredUpdatedAt(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL)
	wid := uuid.New()
	old := time.Now().Add(-time.Hour).UTC()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{
		ID: wid, Name: "Kiosk", KioskBackgroundPosX: "center", KioskBackgroundPosY: "center",
		CreatedAt: old, UpdatedAt: old,
	}).Error)

	h := NewWeddingHandler(services.NewWeddingService(repository.NewWeddingRepo(db)), repository.NewAdminRepo(db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{id}/kiosk", h.UpdateKioskSettings)
	fuego.Get(s, "/api/weddings/{id}", h.Get)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, withAdmin(req))
		return rec
	}
	updatedAt := func(rec *httptest.ResponseRecorder) string {
		t.Helper()
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response %q: %v", rec.Body.String(), err)
		}
		value, ok := got["updatedAt"].(string)
		if !ok {
			t.Fatalf("response has no updatedAt: %s", rec.Body.String())
		}
		return value
	}

	id := utils.EncodeUUID(wid)
	rec := do(http.MethodPut, "/api/weddings/"+id+"/kiosk", `{"kioskBackgroundPosX":"37%"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body %s", rec.Code, rec.Body.String())
	}
	putAt := updatedAt(rec)

	rec = do(http.MethodGet, "/api/weddings/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body %s", rec.Code, rec.Body.String())
	}
	if getAt := updatedAt(rec); putAt != getAt {
		t.Fatalf("PUT updatedAt = %s, follow-up GET = %s; response must carry the stored value", putAt, getAt)
	}
	if putAt == old.Format(time.RFC3339Nano) {
		t.Fatalf("updatedAt = %s, want the write to bump it past the seeded %s", putAt, old)
	}
}
