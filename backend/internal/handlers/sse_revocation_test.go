package handlers

import (
	"bufio"
	"context"
	"io"
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
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"
)

const sseTestSecret = "test-secret"

// newFastSSEServer is newTestSSEServer with a 20ms keepalive, so the
// revocation re-check runs quickly. It returns the auth service for revoking.
func newFastSSEServer(t *testing.T) (*httptest.Server, *services.AuthService) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	hub := services.NewSSEHub(rdb)
	t.Cleanup(hub.Shutdown)

	auth := services.NewAuthService(nil, nil, nil, sseTestSecret, rdb)
	h := NewSSEHandler(hub, auth)
	h.keepalive = 20 * time.Millisecond

	widStr := utils.EncodeUUID(uuid.New())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("wid", widStr)
		h.StreamHandler()(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, auth
}

func mintSSEToken(t *testing.T, pwc bool) (string, *services.AccessClaims) {
	t.Helper()
	claims := &services.AccessClaims{
		AdminID:             uuid.New(),
		Role:                "admin",
		ForcePasswordChange: pwc,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(sseTestSecret))
	testutil.NoErr(t, err)
	return token, claims
}

func openStream(t *testing.T, ctx context.Context, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	testutil.NoErr(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoErr(t, err)
	t.Cleanup(func() {
		resp.Body.Close() //nolint:errcheck // can't fail in a way the test can act on
	})
	return resp
}

// countPingsUntilEOF reads the stream until it ends or the deadline passes,
// returning the ping count and whether the server closed the stream.
func countPingsUntilEOF(body io.ReadCloser, deadline time.Duration) (pings int, closed bool) {
	result := make(chan int, 1)
	go func() {
		n := 0
		sc := bufio.NewScanner(body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), ": ping") {
				n++
			}
		}
		result <- n
	}()
	select {
	case n := <-result:
		return n, true
	case <-time.After(deadline):
		body.Close() //nolint:errcheck // unblocks the scanner; error has no bearing on the result
		return <-result, false
	}
}

// A stream whose token is revoked after connect must be closed by the server
// on the next keepalive tick instead of streaming guest events forever.
func TestSSEStream_ClosesOnRevocation(t *testing.T) {
	ts, auth := newFastSSEServer(t)
	token, claims := mintSSEToken(t, false)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp := openStream(t, ctx, ts.URL, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	testutil.NoErr(t, auth.BlacklistAccessToken(ctx, claims))

	if _, closed := countPingsUntilEOF(resp.Body, 2*time.Second); !closed {
		t.Fatal("stream still open after token was revoked")
	}
}

// Control: an unrevoked stream keeps pinging and stays open.
func TestSSEStream_StaysOpenWhileValid(t *testing.T) {
	ts, _ := newFastSSEServer(t)
	token, _ := mintSSEToken(t, false)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp := openStream(t, ctx, ts.URL, token)

	pings, closed := countPingsUntilEOF(resp.Body, 300*time.Millisecond)
	if closed {
		t.Fatal("valid stream was closed")
	}
	if pings < 2 {
		t.Fatalf("got %d pings in 300ms with a 20ms keepalive, want several", pings)
	}
}

func TestSSEStream_RejectsForcePasswordChangeToken(t *testing.T) {
	ts, _ := newFastSSEServer(t)
	token, _ := mintSSEToken(t, true)

	resp := openStream(t, t.Context(), ts.URL, token)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}
