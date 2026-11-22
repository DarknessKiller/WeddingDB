package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// Offline sync invariant 8: one bad mutation fails alone.
func TestSync_MalformedMutationFailsAlone(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/sync", NewGuestHandler(services.NewGuestService(nil, nil, nil, nil)).Sync)

	// The second item decodes but carries an unusable id, so it is answered
	// without a database; it proves the batch kept going past the bad item.
	body := `{"mutations":[{"op":123},{"op":"update","guestId":"bad","clientUpdatedAt":"2026-01-01T00:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(uuid.New())+"/guests/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Results []services.SyncResult `json:"results"`
	}
	testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &out))
	if len(out.Results) != 2 {
		t.Fatalf("results = %+v, want 2", out.Results)
	}
	if r := out.Results[0]; r.Status != "failed" || r.Retryable == nil || *r.Retryable {
		t.Fatalf("malformed result = %+v, want failed with retryable false", r)
	}
	if r := out.Results[1]; r.Status != "skipped" {
		t.Fatalf("second result = %+v, want it processed", r)
	}
}
