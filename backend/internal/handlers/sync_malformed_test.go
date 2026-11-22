package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
)

// Offline sync invariant 8: one bad mutation fails alone.
func TestSync_MalformedMutationFailsAlone(t *testing.T) {
	// Through RegisterRoutes: auth, wedding scope and the 1 MiB body limit all run.
	a := newAuthRoutes(t)
	// The second item decodes but carries an unusable id, so it is answered
	// without a database; it proves the batch kept going past the bad item.
	body := `{"mutations":[{"op":123},{"op":"update","guestId":"bad","clientUpdatedAt":"2026-01-01T00:00:00Z"}]}`
	rec := a.post("/api/weddings/"+utils.EncodeUUID(uuid.New())+"/guests/sync", a.token(t, uuid.New(), "admin"), body)
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

// The client chunks at 100 mutations and the server accepts exactly that: one
// batch over the chunk size is rejected before any mutation is applied.
func TestSync_RejectsMoreThanChunk(t *testing.T) {
	a := newAuthRoutes(t)
	mutations := func(n int) string {
		items := make([]string, 0, n)
		for range n {
			items = append(items, `{"op":"update","guestId":"bad","clientUpdatedAt":"2026-01-01T00:00:00Z"}`)
		}
		return `{"mutations":[` + strings.Join(items, ",") + `]}`
	}
	token := a.token(t, uuid.New(), "admin")
	path := "/api/weddings/" + utils.EncodeUUID(uuid.New()) + "/guests/sync"
	if rec := a.post(path, token, mutations(maxSyncMutations+1)); rec.Code != http.StatusBadRequest {
		t.Fatalf("%d mutations status = %d, body %s", maxSyncMutations+1, rec.Code, rec.Body.String())
	}
	if rec := a.post(path, token, mutations(maxSyncMutations)); rec.Code != http.StatusOK {
		t.Fatalf("%d mutations status = %d, body %s", maxSyncMutations, rec.Code, rec.Body.String())
	}
}
