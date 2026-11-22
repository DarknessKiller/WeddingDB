package handlers

import (
	"net/http"
	"strings"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Login must only 401 real credential failures. A DB or Redis outage is 503 so
// the client keeps its session, and no driver text reaches the body.
func TestLogin_CredentialRejectionVsOutage(t *testing.T) {
	t.Run("bad credentials", func(t *testing.T) {
		a := newAuthRoutes(t)
		rec := a.post("/api/auth/login", "", `{"email":"missing@x.test","password":"wrong"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401, body %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "invalid credentials") {
			t.Fatalf("raw internal error leaked: %s", rec.Body.String())
		}
	})

	t.Run("database outage", func(t *testing.T) {
		a := newAuthRoutes(t)
		sqlDB, err := a.db.DB()
		testutil.NoErr(t, err)
		testutil.NoErr(t, sqlDB.Close())
		rec := a.post("/api/auth/login", "", `{"email":"missing@x.test","password":"wrong"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503, body %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "database is closed") {
			t.Fatalf("driver error leaked: %s", rec.Body.String())
		}
	})

	t.Run("redis outage", func(t *testing.T) {
		a := newAuthRoutes(t)
		hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
		testutil.NoErr(t, err)
		testutil.NoErr(t, a.db.Create(&models.AdminUser{
			ID: uuid.New(), Email: "admin@x.test", Password: string(hash), Role: "admin",
		}).Error)
		a.mr.Close()
		rec := a.post("/api/auth/login", "", `{"email":"admin@x.test","password":"correct-horse"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503, body %s", rec.Code, rec.Body.String())
		}
	})
}
