package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
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
	for _, field := range []string{"phone", "email", "checkedInAt", "rsvp", "notes"} {
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

// TestPublicGuestList_InvalidCursorIs400 covers the unauthenticated list path:
// a client mistake must not surface as a generic 500.
func TestPublicGuestList_InvalidCursorIs400(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/public/weddings/{wid}/guests", NewPublicGuestHandler(svc).List)

	req := httptest.NewRequest(http.MethodGet, "/api/public/weddings/"+utils.EncodeUUID(uuid.New())+"/guests?cursor=not-a-cursor", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
}
