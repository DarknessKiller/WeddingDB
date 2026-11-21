package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"
)

// An unassign whose token revoke failed leaves the user's token valid; the
// open stream must still close once the DB assignment is gone, or it keeps
// receiving that wedding's guest events.
func TestSSEStream_ClosesOnUnassign(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, wedding_id TEXT NOT NULL, created_at DATETIME)`).Error)
	user, wid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.UserWedding{ID: uuid.New(), UserID: user, WeddingID: wid}).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	testutil.NoErr(t, mr.Set("user:"+user.String()+":tv", "0"))
	hub := services.NewSSEHub(rdb)
	t.Cleanup(hub.Shutdown)
	auth := services.NewAuthService(nil, nil, nil, sseTestSecret, rdb)
	h := NewSSEHandler(hub, auth, repository.NewAdminRepo(db))
	h.keepalive = 20 * time.Millisecond
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("wid", utils.EncodeUUID(wid))
		h.StreamHandler()(w, r)
	}))
	t.Cleanup(ts.Close)

	claims := &services.AccessClaims{
		AdminID:   user,
		Role:      "user",
		WeddingID: &wid,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(sseTestSecret))
	testutil.NoErr(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp := openStream(t, ctx, ts.URL, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if _, closed := countPingsUntilEOF(resp.Body, 100*time.Millisecond); closed {
		t.Fatal("stream closed while still assigned")
	}

	resp = openStream(t, ctx, ts.URL, token)
	testutil.NoErr(t, db.Where("user_id = ?", user).Delete(&models.UserWedding{}).Error)
	if _, closed := countPingsUntilEOF(resp.Body, 2*time.Second); !closed {
		t.Fatal("stream still open after user was unassigned")
	}
}
