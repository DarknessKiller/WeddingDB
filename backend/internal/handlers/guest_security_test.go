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
	guest := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
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
