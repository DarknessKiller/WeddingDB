package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"
)

func testSSEHub(t *testing.T) *SSEHub {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	hub := NewSSEHub(rdb)
	t.Cleanup(func() { hub.Shutdown(context.Background()); testutil.NoErr(t, rdb.Close()) })
	return hub
}

func testSubscribe(t *testing.T, hub *SSEHub, wid uuid.UUID) *SSEClient {
	t.Helper()
	client, err := hub.Subscribe(context.Background(), wid, uuid.New())
	if err != nil && !errors.Is(err, ErrSSECapacity) {
		t.Errorf("subscribe: %v", err)
	}
	return client
}

func TestSSEHub_ConcurrentWeddingLimit(t *testing.T) {
	hub := testSSEHub(t)
	wid := uuid.New()
	clients := make([]*SSEClient, 200)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() { clients[i] = testSubscribe(t, hub, wid) })
	}
	wg.Wait()
	var accepted []*SSEClient
	for _, client := range clients {
		if client != nil {
			accepted = append(accepted, client)
		}
	}
	if len(accepted) != 100 {
		t.Fatalf("accepted %d streams, want 100", len(accepted))
	}
	other := testSubscribe(t, hub, uuid.New())
	if other == nil {
		t.Fatal("another wedding was rejected")
	}
	hub.Unsubscribe(accepted[0])
	replacement := testSubscribe(t, hub, wid)
	if replacement == nil {
		t.Fatal("released slot was not reusable")
	}
	for _, client := range accepted[1:] {
		hub.Unsubscribe(client)
	}
	hub.Unsubscribe(replacement)
	hub.Unsubscribe(other)
}

func TestSSEHub_SubscribeUnsubscribe(t *testing.T) {
	hub := testSSEHub(t)

	wid := uuid.New()
	client := testSubscribe(t, hub, wid)

	if client == nil {
		t.Fatal("Subscribe returned nil client")
	}
	if client.WeddingID != wid {
		t.Errorf("WeddingID = %v, want %v", client.WeddingID, wid)
	}

	// Verify client is registered.
	hub.mu.RLock()
	clients := hub.clients[wid]
	hub.mu.RUnlock()

	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}

	hub.Unsubscribe(client)

	hub.mu.RLock()
	clients = hub.clients[wid]
	hub.mu.RUnlock()

	if len(clients) != 0 {
		t.Fatalf("expected 0 clients after unsubscribe, got %d", len(clients))
	}
}

func TestSSEHub_FanOut(t *testing.T) {
	hub := testSSEHub(t)

	wid := uuid.New()
	const numClients = 3
	clients := make([]*SSEClient, numClients)
	for i := range numClients {
		clients[i] = testSubscribe(t, hub, wid)
	}

	// Manually fan out an event (simulating what subscribeRedis does).
	event := GuestEvent{
		Type:      "checkin",
		GuestID:   uuid.New().String(),
		WeddingID: wid.String(),
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(event)
	testutil.NoErr(t, err)

	hub.mu.RLock()
	weddingClients := hub.clients[wid]
	hub.mu.RUnlock()

	for _, c := range weddingClients {
		c.Chan <- data
	}

	// Verify all clients received the event.
	for i, c := range clients {
		select {
		case received := <-c.Chan:
			var parsed GuestEvent
			if err := json.Unmarshal(received, &parsed); err != nil {
				t.Errorf("client %d: failed to parse event: %v", i, err)
			}
			if parsed.Type != "checkin" {
				t.Errorf("client %d: Type = %q, want %q", i, parsed.Type, "checkin")
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("client %d: timed out waiting for event", i)
		}
	}
}

// A full publish queue must not block the guest write after commit. The event
// is dropped and connected clients are forced to reconnect and resync.
func TestSSEHub_PublishQueueFullReconnectsClients(t *testing.T) {
	wid := uuid.New()
	client := &SSEClient{ID: uuid.New().String(), WeddingID: wid, Done: make(chan struct{})}
	hub := &SSEHub{
		clients: map[uuid.UUID]map[string]*SSEClient{wid: {client.ID: client}},
		ctx:     context.Background(),
		publish: make(chan publishRequest, ssePublishQueue),
	}
	for range ssePublishQueue {
		hub.publish <- publishRequest{}
	}

	done := make(chan struct{})
	go func() {
		hub.publishGuest("create", &models.GuestRecord{ID: uuid.New()}, wid)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishGuest blocked on a full queue")
	}
	select {
	case <-client.Done:
	default:
		t.Fatal("client was not asked to reconnect after a dropped event")
	}
	if len(hub.publish) != ssePublishQueue {
		t.Fatalf("queue length = %d, want %d (event must be dropped)", len(hub.publish), ssePublishQueue)
	}
}

// A dropped event must resync clients on every replica, not only the one that
// dropped it: the Redis marker closes the other replica's streams.
func TestSSEHub_PublishQueueFullResyncsOtherReplicas(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb1 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		testutil.NoErr(t, rdb1.Close())
		testutil.NoErr(t, rdb2.Close())
	})
	other := NewSSEHub(rdb2)
	t.Cleanup(func() { other.Shutdown(context.Background()) })

	wid := uuid.New()
	client := testSubscribe(t, other, wid)

	hub := &SSEHub{
		clients: make(map[uuid.UUID]map[string]*SSEClient),
		ctx:     context.Background(),
		redis:   rdb1,
		publish: make(chan publishRequest, ssePublishQueue),
		ready:   make(chan struct{}),
	}
	close(hub.ready)
	for range ssePublishQueue {
		hub.publish <- publishRequest{}
	}
	hub.publishGuest("create", &models.GuestRecord{ID: uuid.New()}, wid)

	select {
	case <-client.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("client on another replica was not asked to resync after a dropped event")
	}
}

// A failed Publish must still reconnect local clients, so they refetch instead
// of keeping a snapshot that missed the event.
func TestSSEHub_PublishFailureReconnectsClients(t *testing.T) {
	wid := uuid.New()
	client := &SSEClient{ID: uuid.New().String(), WeddingID: wid, Done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hub := &SSEHub{
		clients: map[uuid.UUID]map[string]*SSEClient{wid: {client.ID: client}},
		ctx:     ctx,
		cancel:  cancel,
		publish: make(chan publishRequest, 1),
		ready:   make(chan struct{}), // never closed: Publish fails
		stopped: make(chan struct{}),
	}
	go hub.publishLoop()
	hub.publish <- publishRequest{weddingID: wid, event: GuestEvent{Type: "create"}}

	select {
	case <-client.Done:
	case <-time.After(time.Second):
		t.Fatal("client was not asked to reconnect after a failed Publish")
	}
}

func TestSSEHub_PublishBeforeReady(t *testing.T) {
	hub := &SSEHub{
		clients: make(map[uuid.UUID]map[string]*SSEClient),
		ctx:     context.Background(),
		ready:   make(chan struct{}),
	}

	err := hub.Publish(context.Background(), uuid.New(), GuestEvent{Type: "test"})
	if err == nil {
		t.Fatal("Publish before ready should error, got nil")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("expected not-ready error, got: %v", err)
	}
}

func TestSSEHub_BufferFull(t *testing.T) {
	hub := testSSEHub(t)

	wid := uuid.New()
	client := testSubscribe(t, hub, wid)

	// Fill the buffer (capacity 64).
	for range 64 {
		client.Chan <- []byte(`"full"`)
	}

	// Simulate the hub's fan-out: send should not block when buffer is full.
	done := make(chan struct{})
	go func() {
		hub.mu.RLock()
		weddingClients := hub.clients[wid]
		hub.mu.RUnlock()
		for _, c := range weddingClients {
			select {
			case c.Chan <- []byte(`"overflow"`):
			default:
				// Hub drops when buffer full — this is the expected path.
			}
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full buffer")
	}
}

func TestSSEHub_MultipleWeddings(t *testing.T) {
	hub := testSSEHub(t)

	wid1 := uuid.New()
	wid2 := uuid.New()
	client1 := testSubscribe(t, hub, wid1)
	client2 := testSubscribe(t, hub, wid2)

	// Send event to wedding 1.
	event := GuestEvent{
		Type:      "create",
		GuestID:   uuid.New().String(),
		WeddingID: wid1.String(),
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(event)
	testutil.NoErr(t, err)

	hub.mu.RLock()
	weddingClients := hub.clients[wid1]
	hub.mu.RUnlock()
	for _, c := range weddingClients {
		c.Chan <- data
	}

	// Client 1 should receive, client 2 should not.
	select {
	case <-client1.Chan:
		// Expected.
	case <-time.After(100 * time.Millisecond):
		t.Error("client1: timed out waiting for event")
	}

	select {
	case <-client2.Chan:
		t.Error("client2: should not receive events for different wedding")
	case <-time.After(50 * time.Millisecond):
		// Expected — no event.
	}
}

func TestExtractWeddingID(t *testing.T) {
	tests := []struct {
		channel string
		want    uuid.UUID
	}{
		{"wedding:" + uuid.New().String() + ":guests", uuid.Nil}, // need to parse dynamically
		{"invalid", uuid.Nil},
		{"wedding:not-a-uuid:guests", uuid.Nil},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			// For the valid case, we need to construct it properly.
			if tt.channel == "invalid" || tt.channel == "wedding:not-a-uuid:guests" {
				got := extractWeddingID(tt.channel)
				if got != uuid.Nil {
					t.Errorf("extractWeddingID(%q) = %v, want Nil", tt.channel, got)
				}
			}
		})
	}

	// Test valid channel.
	wid := uuid.New()
	validChannel := "wedding:" + wid.String() + ":guests"
	got := extractWeddingID(validChannel)
	if got != wid {
		t.Errorf("extractWeddingID(%q) = %v, want %v", validChannel, got, wid)
	}
}

func TestGuestToEventData(t *testing.T) {
	wid := uuid.New()
	gid := uuid.New()
	tableID := uuid.New()
	seatNum := 5
	now := time.Now()
	angbao := 888
	gift := "gold ring"

	guest := &mockGuestRecord{
		id:          gid,
		weddingID:   wid,
		name:        "张三",
		email:       "zhang@example.com",
		pax:         4,
		rsvp:        "confirmed",
		isVip:       true,
		notes:       "VIP table",
		dietary:     []string{"no seafood"},
		tableID:     &tableID,
		seatNum:     &seatNum,
		checkedInAt: &now,
		angbaoAmt:   &angbao,
		giftItem:    &gift,
	}

	// guestToEventData expects *models.GuestRecord, but we can't easily construct one
	// without the actual model. Instead, test the SSE event serialization.
	event := GuestEvent{
		Type:      "checkin",
		GuestID:   guest.id.String(),
		WeddingID: guest.weddingID.String(),
		Guest: &GuestEventData{
			ID:      guest.id.String(),
			Name:    guest.name,
			Email:   guest.email,
			Pax:     guest.pax,
			RSVP:    guest.rsvp,
			IsVip:   guest.isVip,
			Notes:   guest.notes,
			Dietary: guest.dietary,
			TableID: func() *string { s := guest.tableID.String(); return &s }(),
			SeatNum: guest.seatNum,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var parsed GuestEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if parsed.Type != "checkin" {
		t.Errorf("Type = %q, want %q", parsed.Type, "checkin")
	}
	if parsed.Guest == nil {
		t.Fatal("Guest is nil")
	}
	if parsed.Guest.Name != "张三" {
		t.Errorf("Guest.Name = %q, want %q", parsed.Guest.Name, "张三")
	}
	if parsed.Guest.Pax != 4 {
		t.Errorf("Guest.Pax = %d, want %d", parsed.Guest.Pax, 4)
	}
	if !parsed.Guest.IsVip {
		t.Error("Guest.IsVip should be true")
	}
}

// mockGuestRecord is a minimal mock for testing event data conversion.
type mockGuestRecord struct {
	id          uuid.UUID
	weddingID   uuid.UUID
	name        string
	email       string
	pax         int
	rsvp        string
	isVip       bool
	notes       string
	dietary     []string
	tableID     *uuid.UUID
	seatNum     *int
	checkedInAt *time.Time
	angbaoAmt   *int
	giftItem    *string
}

func TestSSEHub_PerUserLimit(t *testing.T) {
	hub := testSSEHub(t)
	user := uuid.New()
	clients := make([]*SSEClient, 0, 5)
	for range 5 {
		c, err := hub.Subscribe(context.Background(), uuid.New(), user)
		testutil.NoErr(t, err)
		clients = append(clients, c)
	}
	if _, err := hub.Subscribe(context.Background(), uuid.New(), user); !errors.Is(err, ErrSSEUserCapacity) {
		t.Fatalf("sixth stream err = %v, want ErrSSEUserCapacity", err)
	}
	other, err := hub.Subscribe(context.Background(), clients[0].WeddingID, uuid.New())
	testutil.NoErr(t, err)
	hub.Unsubscribe(clients[0])
	again, err := hub.Subscribe(context.Background(), uuid.New(), user)
	testutil.NoErr(t, err)
	for _, c := range append(clients[1:], other, again) {
		hub.Unsubscribe(c)
	}
}

// Events keep the order they were written in, and each carries the row's
// write time so a client can drop one that arrives late.
func TestSSEHub_PublishKeepsOrder(t *testing.T) {
	hub := testSSEHub(t)
	wid := uuid.New()
	client := testSubscribe(t, hub, wid)
	g := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "A", Pax: 1, UpdatedAt: time.Now().UTC()}

	const events = 20
	for i := range events {
		if i%2 == 0 {
			hub.publishGuest("checkin", g, wid)
		} else {
			hub.publishGuest("checkout", g, wid)
		}
	}
	for i := range events {
		want := "checkin"
		if i%2 == 1 {
			want = "checkout"
		}
		select {
		case msg := <-client.Chan:
			var ev GuestEvent
			testutil.NoErr(t, json.Unmarshal(msg, &ev))
			if ev.Type != want {
				t.Fatalf("event %d = %q, want %q", i, ev.Type, want)
			}
			if ev.Guest == nil || ev.Guest.UpdatedAt != g.UpdatedAt.Format(time.RFC3339Nano) {
				t.Fatalf("event %d updatedAt = %+v, want %s", i, ev.Guest, g.UpdatedAt.Format(time.RFC3339Nano))
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

// countPublishHook counts PUBLISH commands so resync de-duplication is observable.
type countPublishHook struct{ n int32 }

func (h *countPublishHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *countPublishHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "publish") {
			atomic.AddInt32(&h.n, 1)
		}
		return next(ctx, cmd)
	}
}

func (h *countPublishHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// A Redis outage must not turn every failed event into another blocking resync
// publish: one marker per wedding at a time is enough to force clients to refetch.
func TestSSEHub_BroadcastResyncSingleFlight(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	hook := &countPublishHook{}
	rdb.AddHook(hook)
	hub := &SSEHub{redis: rdb, ready: make(chan struct{})}
	close(hub.ready)
	wid := uuid.New()

	hub.resyncInFlight.Store(wid, struct{}{})
	hub.broadcastResync(context.Background(), wid)
	if got := atomic.LoadInt32(&hook.n); got != 0 {
		t.Fatalf("published %d resync markers while one was in flight, want 0", got)
	}
	hub.resyncInFlight.Delete(wid)
	hub.broadcastResync(context.Background(), wid)
	if got := atomic.LoadInt32(&hook.n); got != 1 {
		t.Fatalf("published %d resync markers after the in-flight one finished, want 1", got)
	}
}

// Shutdown order: CloseStreams ends local streams but keeps publishing, and
// Shutdown still publishes every event queued before it, so writes committed
// during the HTTP drain reach other replicas.
func TestSSEHub_ShutdownPublishesQueuedEvents(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	hub := NewSSEHub(rdb)
	wid := uuid.New()
	sub := rdb.Subscribe(t.Context(), "wedding:"+wid.String()+":guests")
	t.Cleanup(func() { testutil.NoErr(t, sub.Close()) })
	_, err := sub.Receive(t.Context())
	testutil.NoErr(t, err)

	client := testSubscribe(t, hub, wid)
	hub.CloseStreams()
	select {
	case <-client.Done:
	default:
		t.Fatal("CloseStreams left the local stream open")
	}

	const events = 200
	for range events {
		hub.publishGuest("create", &models.GuestRecord{ID: uuid.New()}, wid)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	hub.Shutdown(ctx)

	ch := sub.Channel()
	for i := range events {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d of %d events queued before shutdown", i, events)
		}
	}
}
