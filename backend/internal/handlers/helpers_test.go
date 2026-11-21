package handlers

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/middleware"

	"github.com/google/uuid"
)

func TestRequireWeddingAccess(t *testing.T) {
	wid, other := uuid.New(), uuid.New()
	tests := []struct {
		name    string
		role    string
		scope   *uuid.UUID
		wantErr bool
	}{
		{"admin reaches any wedding", "admin", nil, false},
		{"user in scope", "user", &wid, false},
		{"user in another wedding", "user", &other, true},
		{"user without scope", "user", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(t.Context(), middleware.RoleKey, tt.role)
			ctx = context.WithValue(ctx, middleware.WeddingIDKey, tt.scope)
			err := requireWeddingAccess(ctx, wid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if _, ok := errors.AsType[forbiddenError](err); tt.wantErr && !ok {
				t.Fatalf("err = %T, want forbiddenError", err)
			}
		})
	}
}

func TestRequireAdmin_Forbidden(t *testing.T) {
	ctx := context.WithValue(t.Context(), middleware.RoleKey, "user")
	if _, ok := errors.AsType[forbiddenError](requireAdmin(ctx)); !ok {
		t.Fatal("non-admin: want forbiddenError")
	}
	ctx = context.WithValue(t.Context(), middleware.RoleKey, "admin")
	if err := requireAdmin(ctx); err != nil {
		t.Fatalf("admin: %v", err)
	}
}
