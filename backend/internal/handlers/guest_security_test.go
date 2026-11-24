package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/config"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegisteredGuestSeatDatabaseErrorIsGeneric(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	const secret = "test-secret"
	auth := services.NewAuthService(nil, nil, nil, secret, rdb)
	guest := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := config.NewFuegoServer(config.Env{Port: "0"}, "test")
	RegisterRoutes(s, auth, guest, nil, nil, nil, nil, nil)
	wid := uuid.New()
	claims := services.AccessClaims{AdminID: uuid.New(), Role: "user", WeddingID: &wid, RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	testutil.NoErr(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests/"+utils.EncodeUUID(uuid.New())+"/seat", strings.NewReader(`{"tableId":"`+utils.EncodeUUID(uuid.New())+`","seatNum":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "guest_records") {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

// Through the real routes: a flagged session is stopped before any guest write,
// and a retried create with the same client id returns the stored guest.
func TestRegisteredGuestCreate_ForcePasswordAndClientID(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	const secret = "test-secret"
	s := config.NewFuegoServer(config.Env{Port: "0"}, "test")
	RegisterRoutes(s, services.NewAuthService(nil, nil, nil, secret, rdb), services.NewGuestService(repository.NewGuestRepo(db), nil, db), nil, nil, nil, nil, nil)
	wid, gid := uuid.New(), uuid.New()
	post := func(fpc bool, name string) *httptest.ResponseRecorder {
		claims := services.AccessClaims{AdminID: uuid.New(), Role: "user", WeddingID: &wid, ForcePasswordChange: fpc, RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		testutil.NoErr(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests", strings.NewReader(`{"id":"`+gid.String()+`","name":"`+name+`","pax":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(true, "Blocked"); rec.Code != http.StatusForbidden {
		t.Fatalf("flagged session = %d %s, want 403", rec.Code, rec.Body.String())
	}
	if rec := post(false, "Alice"); rec.Code != http.StatusCreated {
		t.Fatalf("first create = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if rec := post(false, "Alice again"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Alice"`) {
		t.Fatalf("retry = %d %s, want 200 with the stored guest", rec.Code, rec.Body.String())
	}
	var n int64
	testutil.NoErr(t, db.Table("guest_records").Count(&n).Error)
	if n != 1 {
		t.Fatalf("guest rows = %d, want 1", n)
	}
}
