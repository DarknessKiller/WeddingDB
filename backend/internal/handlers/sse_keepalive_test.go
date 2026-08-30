package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"weddingdb/internal/services"
	"weddingdb/internal/utils"
)

// newTestSSEServer spins up an SSEHub on miniredis plus a StreamHandler server
// with a known-good admin JWT so tests can connect as EventSource would.
func newTestSSEServer(t *testing.T) (*httptest.Server, *services.SSEHub, string, uuid.UUID) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	hub := services.NewSSEHub(rdb)
	t.Cleanup(hub.Shutdown)

	// Mint admin JWT with the same secret the handler validates against.
	secret := "test-secret"
	claims := services.AccessClaims{
		AdminID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Role:         "admin",
		TokenVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	auth := services.NewAuthService(nil, nil, nil, secret, rdb)
	h := NewSSEHandler(hub, auth)

	testWID := uuid.New()
	widStr := utils.EncodeUUID(testWID)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("wid", widStr)
		h.StreamHandler()(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, hub, token, testWID
}

// TestSSEKeepalive ensures the stream emits comment pings periodically instead
// of staying silent until a proxy/NAT idle timeout (~60s) kills the stream.
func TestSSEKeepalive(t *testing.T) {
	ts, _, token, wid := newTestSSEServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequest("GET", ts.URL+"/api/weddings/"+wid.String()+"/events?token="+token, nil)
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	// Read bytes until we see the ": ping" comment; ticker fires at 20s but any
	// arrival under a 25s deadline proves periodic keepalive works.
	deadline := time.Now().Add(25 * time.Second)
	var sb strings.Builder
	tmp := make([]byte, 64)
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			sb.Write(tmp[:n])
			if strings.Contains(sb.String(), ": ping") {
				return // pass
			}
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	t.Fatalf("no keepalive ping within deadline; got %q", sb.String())
}
