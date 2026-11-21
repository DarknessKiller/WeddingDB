package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/utils"

	"github.com/google/uuid"
)

func TestWeddingScopeMiddleware(t *testing.T) {
	wid, other := uuid.New(), uuid.New()
	handler := WeddingScopeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tests := []struct {
		name    string
		role    string
		scope   *uuid.UUID
		pathWid string
		want    int
	}{
		{"admin any wedding", "admin", nil, other.String(), http.StatusOK},
		{"user own wedding", "user", &wid, wid.String(), http.StatusOK},
		{"user own wedding base64", "user", &wid, utils.EncodeUUID(wid), http.StatusOK},
		{"user other wedding", "user", &wid, other.String(), http.StatusForbidden},
		{"user without scope", "user", nil, wid.String(), http.StatusForbidden},
		{"invalid wedding id", "user", &wid, "not-an-id", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/weddings/x/guests", nil)
			req.SetPathValue("wid", tt.pathWid)
			ctx := context.WithValue(req.Context(), RoleKey, tt.role)
			ctx = context.WithValue(ctx, WeddingIDKey, tt.scope)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req.WithContext(ctx))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
