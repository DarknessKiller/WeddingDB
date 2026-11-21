package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
)

func TestExportAngpao_Status(t *testing.T) {
	db := newGuestTestDB(t)
	known := uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO wedding_events (id, name) VALUES (?, 'W')`, known).Error)
	h := NewReportHandler(services.NewReportService(repository.NewGuestRepo(db), repository.NewTableRepo(db), repository.NewWeddingRepo(db)))

	tests := []struct {
		name string
		wid  uuid.UUID
		want int
	}{
		{"unknown wedding", uuid.New(), http.StatusNotFound},
		{"known wedding", known, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/weddings/x/reports/angpao", nil)
			req.SetPathValue("wid", tt.wid.String())
			rec := httptest.NewRecorder()
			h.ExportAngpao(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
