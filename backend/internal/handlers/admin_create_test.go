package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

func adminCreateRequest(t *testing.T, s *fuego.Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.RoleKey, "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	return rec
}

// A grant failure must roll back the user row: the old two-write path left a
// user without the requested wedding access.
func TestAdminCreate_GrantFailureLeavesNoUser(t *testing.T) {
	db := newSQLite(t, adminUsersDDL) // user_weddings deliberately absent
	repo := repository.NewAdminRepo(db)
	h := NewAdminHandler(repo, nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/admin/users", h.Create)

	body := `{"email":"a@example.com","password":"NewPassw0rd!","name":"A","weddings":["` + uuid.New().String() + `"]}`
	rec := adminCreateRequest(t, s, body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.AdminUser{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("grant failure left %d user rows, want 0", count)
	}
}

// A duplicate wedding id in the payload is deduped before insert, so the grant
// index never turns it into an "email already registered" conflict.
func TestAdminCreate_DuplicateWeddingIDIsNotEmailConflict(t *testing.T) {
	db := newSQLite(t, adminUsersDDL, userWeddingsDDL)
	testutil.NoErr(t, db.Exec("CREATE UNIQUE INDEX idx_user_wedding ON user_weddings (user_id, wedding_id)").Error)
	h := NewAdminHandler(repository.NewAdminRepo(db), nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/admin/users", h.Create)

	wid := uuid.New()
	body := `{"email":"dup-wedding@example.com","password":"NewPassw0rd!","name":"Dup","weddings":["` +
		wid.String() + `","` + wid.String() + `"]}`
	rec := adminCreateRequest(t, s, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body %s", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.UserWedding{}).Where("wedding_id = ?", wid).Count(&count).Error)
	if count != 1 {
		t.Fatalf("grants = %d, want 1", count)
	}
}

// A concurrent duplicate email hits the unique index after both lookups pass;
// that must be 409, not 500.
func TestAdminCreate_ConcurrentDuplicateEmailIs409(t *testing.T) {
	db := newSQLite(t, adminUsersDDL, userWeddingsDDL)
	testutil.NoErr(t, db.Exec("CREATE UNIQUE INDEX idx_admin_users_email ON admin_users (email)").Error)
	repo := repository.NewAdminRepo(db)
	h := NewAdminHandler(repo, nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/admin/users", h.Create)

	body := `{"email":"dup@example.com","password":"NewPassw0rd!","name":"Dup"}`
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Go(func() { codes[i] = adminCreateRequest(t, s, body).Code })
	}
	wg.Wait()

	want := map[int]int{http.StatusCreated: 1, http.StatusConflict: 1}
	got := map[int]int{}
	for _, code := range codes {
		got[code]++
	}
	for code, n := range want {
		if got[code] != n {
			t.Fatalf("statuses = %v, want one 201 and one 409", codes)
		}
	}
}
