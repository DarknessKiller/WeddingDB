package handlers

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Unassigning a user must take effect on the next request, not when their
// token expires: that check is the only thing between them and the roster.
func TestRequireWeddingAccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, wedding_id TEXT NOT NULL,
		created_at DATETIME, UNIQUE(user_id, wedding_id))`).Error)
	repo := repository.NewAdminRepo(db)

	user := uuid.New()
	scoped := uuid.New()
	unassigned := uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO user_weddings (id, user_id, wedding_id, created_at) VALUES (?, ?, ?, datetime('now'))`,
		uuid.New(), user, scoped).Error)

	ctxFor := func(role string, scope *uuid.UUID, admin uuid.UUID) context.Context {
		ctx := context.WithValue(context.Background(), middleware.RoleKey, role)
		ctx = context.WithValue(ctx, middleware.AdminIDKey, admin)
		if scope != nil {
			ctx = context.WithValue(ctx, middleware.WeddingIDKey, scope)
		}
		return ctx
	}

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		wid     uuid.UUID
		wantErr bool
	}{
		{"admin opens any wedding", ctxFor("admin", nil, user), unassigned, false},
		{"assigned user opens their wedding", ctxFor("user", &scoped, user), scoped, false},
		{"user was unassigned after the token was minted", ctxFor("user", &scoped, user), unassigned, true},
		{"token scoped to another wedding", ctxFor("user", &unassigned, user), scoped, true},
		{"token carries no wedding scope", ctxFor("user", nil, user), scoped, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireWeddingAccess(tc.ctx, repo, tc.wid)
			if !tc.wantErr {
				testutil.NoErr(t, err)
				return
			}
			var forbidden fuego.ForbiddenError
			if !errors.As(err, &forbidden) {
				t.Fatalf("err = %v, want a ForbiddenError", err)
			}
		})
	}
}
