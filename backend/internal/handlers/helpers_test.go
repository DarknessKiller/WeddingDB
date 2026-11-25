package handlers

import (
	"context"
	"testing"
	"weddingdb/internal/middleware"

	"github.com/google/uuid"
)

func TestRequireWeddingAccess(t *testing.T) {
	wedding := uuid.New()
	other := uuid.New()

	ctxWith := func(role string, wid *uuid.UUID) context.Context {
		ctx := context.WithValue(context.Background(), middleware.RoleKey, role)
		if wid != nil {
			ctx = context.WithValue(ctx, middleware.WeddingIDKey, wid)
		}
		return ctx
	}

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		wantErr bool
	}{
		{"admin any wedding", ctxWith("admin", nil), false},
		{"user own wedding", ctxWith("user", &wedding), false},
		{"user other wedding", ctxWith("user", &other), true},
		{"user no wedding", ctxWith("user", nil), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireWeddingAccess(tc.ctx, wedding)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
