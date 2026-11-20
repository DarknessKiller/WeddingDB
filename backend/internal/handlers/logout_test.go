package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Logout is public and unauthenticated, so it must share the auth rate limit;
// otherwise anyone can churn the refresh-token store at will.
func TestLogout_RateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/auth/logout", NewAuthHandler(authSvc, adminRepo).Logout)

	// Pre-seed the current per-second bucket at the limit. Retry if the
	// wall-clock second ticks between seeding and the request.
	limited := false
	for attempt := 0; attempt < 3 && !limited; attempt++ {
		bucket := fmt.Sprintf("ratelimit:auth:ip:192.0.2.1:%d", time.Now().Unix())
		testutil.NoErr(t, mr.Set(bucket, "25"))
		mr.SetTTL(bucket, time.Second)

		rec := serveGuest(t, s, http.MethodPost, "/api/auth/logout", `{"refreshToken":""}`)
		limited = rec.Code == http.StatusTooManyRequests
	}
	if !limited {
		t.Fatal("logout must be rate limited once the auth bucket is full")
	}
}
