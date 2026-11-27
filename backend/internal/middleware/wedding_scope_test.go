package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func newScopeRequest(t *testing.T, role string, jwtWid *uuid.UUID, urlWid string) *httptest.ResponseRecorder {
	t.Helper()
	h := WeddingScopeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/weddings/"+urlWid+"/guests", nil)
	req.SetPathValue("wid", urlWid)
	ctx := req.Context()
	if role != "" {
		ctx = context.WithValue(ctx, RoleKey, role)
	}
	if jwtWid != nil {
		ctx = context.WithValue(ctx, WeddingIDKey, jwtWid)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func TestWeddingScope_UserScopedToOwnWedding(t *testing.T) {
	wid := uuid.New()
	if rec := newScopeRequest(t, "user", &wid, wid.String()); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestWeddingScope_UserRejectedForOtherWedding(t *testing.T) {
	own, other := uuid.New(), uuid.New()
	if rec := newScopeRequest(t, "user", &own, other.String()); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestWeddingScope_AdminAllowedAnyWedding(t *testing.T) {
	if rec := newScopeRequest(t, "admin", nil, uuid.New().String()); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestWeddingScope_UserWithNoWeddingRejected(t *testing.T) {
	if rec := newScopeRequest(t, "user", nil, uuid.New().String()); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestWeddingScope_MalformedWeddingID(t *testing.T) {
	wid := uuid.New()
	if rec := newScopeRequest(t, "user", &wid, "not-a-wedding"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
