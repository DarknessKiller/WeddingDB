package handlers

import (
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

// newTestSSEServer spins up an SSEHub on miniredis plus a StreamHandler server
// with a known-good admin JWT so tests can connect as EventSource would.
func newTestSSEServer(t *testing.T) (*httptest.Server, *services.SSEHub, string, uuid.UUID) {
	ts, hub, _, token, wid := newTestSSEServerWithLeaseClient(t)
	return ts, hub, token, wid
}

func newTestSSEServerWithLeaseClient(t *testing.T) (*httptest.Server, *services.SSEHub, *redis.Client, string, uuid.UUID) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	leaseRedis := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	t.Cleanup(func() {
		if err := leaseRedis.Close(); err != nil && err != redis.ErrClosed {
			t.Errorf("close lease Redis: %v", err)
		}
	})
	hub := services.NewSSEHub(leaseRedis)
	t.Cleanup(func() { hub.Shutdown(context.Background()) })

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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/weddings/{wid}/events", h.StreamHandler())
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, hub, leaseRedis, token, testWID
}

func TestSSEAdmissionRedisFailureReturns503(t *testing.T) {
	ts, _, leaseRedis, token, wid := newTestSSEServerWithLeaseClient(t)
	testutil.NoErr(t, leaseRedis.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/weddings/"+utils.EncodeUUID(wid)+"/events", nil)
	testutil.NoErr(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoErr(t, err)
	defer func() { testutil.NoErr(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("failure sent stream headers")
	}
}

func TestSSERenewalRedisFailureClosesStream(t *testing.T) {
	ts, _, leaseRedis, token, wid := newTestSSEServerWithLeaseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/weddings/"+utils.EncodeUUID(wid)+"/events", nil)
	testutil.NoErr(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoErr(t, err)
	defer func() { testutil.NoErr(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream response: status %d, content-type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	initial := make([]byte, len(": keepalive\n\n"))
	_, err = io.ReadFull(resp.Body, initial)
	testutil.NoErr(t, err)
	if string(initial) != ": keepalive\n\n" {
		t.Fatalf("initial stream data = %q", initial)
	}
	testutil.NoErr(t, leaseRedis.Close())
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("stream did not close cleanly after renewal failure: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("unexpected data after Redis failure: %q", data)
	}
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

// The stream is mounted outside AuthMiddleware, so it must apply the
// forced-password-change gate itself.
func TestSSERejectsForcePasswordChange(t *testing.T) {
	ts, _, _, wid := newTestSSEServer(t)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, services.AccessClaims{
		AdminID:             uuid.New(),
		Role:                "admin",
		ForcePasswordChange: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "fpc-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte("test-secret"))
	testutil.NoErr(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/api/weddings/"+utils.EncodeUUID(wid)+"/events", nil)
	testutil.NoErr(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoErr(t, err)
	defer func() { testutil.NoErr(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestSSEWeddingStreamLimit(t *testing.T) {
	ts, hub, token, wid := newTestSSEServer(t)
	clients := make([]*services.SSEClient, 99)
	var err error
	for i := range clients {
		clients[i], err = hub.Subscribe(context.Background(), wid, uuid.New())
		testutil.NoErr(t, err)
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
		client, err := hub.Subscribe(context.Background(), wid, uuid.New())
		if err == nil {
			hub.Unsubscribe(client)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnect did not release slot")
		}
		time.Sleep(time.Millisecond)
	}
}
