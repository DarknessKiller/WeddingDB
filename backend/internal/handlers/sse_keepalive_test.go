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
	"weddingdb/internal/testutil"
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
	req, err := http.NewRequest("GET", ts.URL+"/api/weddings/"+wid.String()+"/events", nil)
	testutil.NoErr(t, err)
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
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

func TestSSERejectsQueryToken(t *testing.T) {
	ts, _, token, wid := newTestSSEServer(t)

	for _, test := range []struct {
		name   string
		query  string
		header string
	}{
		{name: "query token", query: "?token=" + token},
		{name: "missing token"},
		{name: "malformed header", header: "Token " + token},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL+"/api/weddings/"+wid.String()+"/events"+test.query, nil)
			testutil.NoErr(t, err)
			if test.header != "" {
				req.Header.Set("Authorization", test.header)
			}

			resp, err := http.DefaultClient.Do(req)
			testutil.NoErr(t, err)
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
			}()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestSSEWeddingStreamLimit(t *testing.T) {
	ts, hub, token, wid := newTestSSEServer(t)
	clients := make([]*services.SSEClient, 99)
	for i := range clients {
		clients[i] = hub.Subscribe(wid)
		if clients[i] == nil {
			t.Fatal("unexpected capacity rejection")
		}
		defer hub.Unsubscribe(clients[i])
	}
	request := func(ctx context.Context) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/weddings/"+wid.String()+"/events", nil)
		testutil.NoErr(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		testutil.NoErr(t, err)
		return resp
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := request(ctx)
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", stream.StatusCode)
	}
	defer func() {
		cancel()
		testutil.NoErr(t, stream.Body.Close())
	}()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	resp := request(ctx2)
	defer func() {
		cancel2()
		testutil.NoErr(t, resp.Body.Close())
	}()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("full wedding status = %d, want 429", resp.StatusCode)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		client := hub.Subscribe(wid)
		if client != nil {
			hub.Unsubscribe(client)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnect did not release slot")
		}
		time.Sleep(time.Millisecond)
	}
}
