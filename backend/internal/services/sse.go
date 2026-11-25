package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GuestEvent represents a guest mutation event for SSE broadcasting.
type GuestEvent struct {
	Type      string          `json:"type"` // "create", "update", "delete", "checkin", "checkout", "seat_assign"
	GuestID   string          `json:"guestId"`
	WeddingID string          `json:"weddingId"`
	Guest     *GuestEventData `json:"guest,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// GuestEventData is the guest payload in an SSE event.
type GuestEventData struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Pax         int      `json:"pax"`
	RSVP        string   `json:"rsvp"`
	IsVip       bool     `json:"isVip"`
	Notes       string   `json:"notes"`
	Dietary     []string `json:"dietary"`
	TableID     *string  `json:"tableId"`
	SeatNum     *int     `json:"seatNum"`
	CheckedInAt *string  `json:"checkedInAt"`
	AngbaoAmt   *int     `json:"angbaoAmt"`
	GiftItem    *string  `json:"giftItem"`
	Pinyin      string   `json:"pinyin"`
	// UpdatedAt is the row's write time, so a client can drop an event that
	// arrives after a newer one (out-of-order delivery, reconnect replay).
	UpdatedAt string `json:"updatedAt"`
}

// SSEClient represents a connected SSE client.
type SSEClient struct {
	ID          string
	WeddingID   uuid.UUID
	UserID      uuid.UUID
	Chan        chan []byte
	Done        chan struct{}
	doneOnce    sync.Once
	releaseOnce sync.Once
	mu          sync.RWMutex
	deadline    time.Time
}

const sseLeaseDuration = 30 * time.Second

// sseResyncType is a control event, not a guest mutation: every replica closes
// its local streams for the wedding so clients reconnect and refetch a snapshot
// that includes an event this deployment could not deliver.
const sseResyncType = "resync"

// KEYS[1] holds the wedding's leases (cap 100), KEYS[2] the user's (cap 20).
// Returns 1 on success, 0 when the wedding is full, -1 when the user is.
var sseAcquire = redis.NewScript(`
local now = redis.call('TIME')
local ms = now[1] * 1000 + math.floor(now[2] / 1000)
for _, k in ipairs(KEYS) do redis.call('ZREMRANGEBYSCORE', k, '-inf', ms) end
if redis.call('ZCARD', KEYS[1]) >= 100 then return 0 end
if redis.call('ZCARD', KEYS[2]) >= 20 then return -1 end
for _, k in ipairs(KEYS) do
  redis.call('ZADD', k, ms + ARGV[2], ARGV[1])
  redis.call('PEXPIRE', k, ARGV[2])
end
return 1`)

var sseRenew = redis.NewScript(`
local now = redis.call('TIME')
local ms = now[1] * 1000 + math.floor(now[2] / 1000)
for _, k in ipairs(KEYS) do
  local expiry = redis.call('ZSCORE', k, ARGV[1])
  if not expiry or tonumber(expiry) <= ms then return 0 end
end
for _, k in ipairs(KEYS) do
  redis.call('ZADD', k, ms + ARGV[2], ARGV[1])
  redis.call('PEXPIRE', k, ARGV[2])
end
return 1`)

func (c *SSEClient) Deadline() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deadline
}

func (c *SSEClient) key() string { return "wedding:" + c.WeddingID.String() + ":sse-leases" }

func (c *SSEClient) userKey() string { return "user:" + c.UserID.String() + ":sse-leases" }

func (c *SSEClient) keys() []string { return []string{c.key(), c.userKey()} }

// SSEHub manages SSE connections per wedding and Redis pub/sub.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[string]*SSEClient // weddingID -> clientID -> client
	redis   *redis.Client
	pubsub  *redis.PubSub
	ctx     context.Context
	cancel  context.CancelFunc
	// publish serializes outgoing events through one goroutine, so a check-in
	// and its undo keep the order they were written in instead of the order two
	// goroutines happened to run.
	publish chan publishRequest
	// ponytail: closed after PSubscribe ACKs so the first Publish is not lost during Redis warm-up.
	// Race B (client joins SSE mid-publish) is not addressed here — fix needs a per-wedding replay buffer.
	ready chan struct{}
	// stopped closes when publishLoop has drained the queue and exited.
	stopped chan struct{}
	// resyncInFlight collapses resync broadcasts during a Redis outage: one
	// marker per wedding at a time, so a broken Redis cannot turn every failed
	// event into another blocking publish.
	resyncInFlight sync.Map
}

// ssePublishQueue bounds the ordered publish queue. A full queue drops the
// event and forces local clients to reconnect, so a slow Redis cannot block
// guest writes.
const ssePublishQueue = 1024

type publishRequest struct {
	weddingID uuid.UUID
	event     GuestEvent
}

// NewSSEHub creates a new SSEHub and starts the Redis subscriber.
func NewSSEHub(rdb *redis.Client) *SSEHub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &SSEHub{clients: make(map[uuid.UUID]map[string]*SSEClient), redis: rdb, ctx: ctx, cancel: cancel, publish: make(chan publishRequest, ssePublishQueue), ready: make(chan struct{}), stopped: make(chan struct{})}
	// Created here, not in the goroutine, so Shutdown reads it without a race.
	// With no pattern yet it makes no network call; subscribeRedis subscribes.
	hub.pubsub = rdb.PSubscribe(ctx)
	go hub.subscribeRedis()
	go hub.publishLoop()
	select {
	case <-hub.ready:
	case <-time.After(5 * time.Second):
		log.Printf("SSE: PSubscribe not ready after 5s; Publish will error until Redis recovers")
	}
	return hub
}

// subscribeRedis listens on the wildcard Redis channel and fans out to local clients.
func (h *SSEHub) subscribeRedis() {
	if err := h.pubsub.PSubscribe(h.ctx, "wedding:*:guests"); err != nil {
		log.Printf("SSE: PSubscribe failed: %v", err)
	}
	// Wait for Redis to confirm the subscription is active
	if err := h.pubsub.Ping(h.ctx); err != nil {
		log.Printf("SSE: PSubscribe ping failed: %v", err)
	}
	close(h.ready)
	ch := h.pubsub.Channel()
	for {
		select {
		case <-h.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			wid := extractWeddingID(msg.Channel)
			if wid == uuid.Nil {
				continue
			}
			if strings.HasPrefix(msg.Payload, `{"type":"`+sseResyncType+`"`) {
				h.reconnectWedding(wid)
				continue
			}
			h.mu.RLock()
			clients := make([]*SSEClient, 0, len(h.clients[wid]))
			for _, client := range h.clients[wid] {
				clients = append(clients, client)
			}
			h.mu.RUnlock()
			for _, client := range clients {
				select {
				case client.Chan <- []byte(msg.Payload):
				default:
					// A dropped event leaves the snapshot stale; force reconnect/resync instead.
					client.doneOnce.Do(func() { close(client.Done) })
				}
			}
		}
	}
}

// Subscribe atomically acquires deployment-wide wedding and user leases before
// registering a client.
func (h *SSEHub) Subscribe(ctx context.Context, weddingID, userID uuid.UUID) (*SSEClient, error) {
	client := &SSEClient{ID: uuid.New().String(), WeddingID: weddingID, UserID: userID, Chan: make(chan []byte, 64), Done: make(chan struct{})}
	start := time.Now()
	result, err := sseAcquire.Run(ctx, h.redis, client.keys(), client.ID, sseLeaseDuration.Milliseconds()).Int()
	if err != nil {
		return nil, fmt.Errorf("acquire SSE lease: %w", err)
	}
	switch result {
	case 0:
		return nil, ErrSSECapacity
	case -1:
		return nil, ErrSSEUserCapacity
	}
	client.deadline = start.Add(sseLeaseDuration)
	if !time.Now().Before(client.deadline) {
		return nil, fmt.Errorf("SSE lease expired during acquisition")
	}
	h.mu.Lock()
	if h.clients[weddingID] == nil {
		h.clients[weddingID] = make(map[string]*SSEClient)
	}
	h.clients[weddingID][client.ID] = client
	h.mu.Unlock()
	log.Printf("SSE: client %s connected for wedding %s", client.ID, weddingID)
	go h.renewLease(client)
	return client, nil
}

func (h *SSEHub) renewLease(client *SSEClient) {
	ticker := time.NewTicker(sseLeaseDuration / 3)
	defer ticker.Stop()
	for {
		select {
		case <-client.Done:
			return
		case <-h.ctx.Done():
			client.doneOnce.Do(func() { close(client.Done) })
			return
		case <-ticker.C:
			start := time.Now()
			ctx, cancel := context.WithDeadline(h.ctx, client.Deadline())
			result, err := sseRenew.Run(ctx, h.redis, client.keys(), client.ID, sseLeaseDuration.Milliseconds()).Int()
			cancel()
			if err != nil || result == 0 || !time.Now().Before(client.Deadline()) {
				client.doneOnce.Do(func() { close(client.Done) })
				return
			}
			client.mu.Lock()
			client.deadline = start.Add(sseLeaseDuration)
			client.mu.Unlock()
		}
	}
}

// Unsubscribe removes an SSE client.
func (h *SSEHub) Unsubscribe(client *SSEClient) {
	h.mu.Lock()
	if clients, ok := h.clients[client.WeddingID]; ok {
		delete(clients, client.ID)
		if len(clients) == 0 {
			delete(h.clients, client.WeddingID)
		}
	}
	h.mu.Unlock()
	client.doneOnce.Do(func() { close(client.Done) })
	client.releaseOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		for _, key := range client.keys() {
			if err := h.redis.ZRem(ctx, key, client.ID).Err(); err != nil {
				log.Printf("SSE: lease release failed: %v", err)
			}
		}
	})
	log.Printf("SSE: client %s disconnected from wedding %s", client.ID, client.WeddingID)
}

// publishLoop drains the publish queue in order. On shutdown it publishes
// what is still queued, so writes committed during the HTTP drain still reach
// other replicas.
func (h *SSEHub) publishLoop() {
	defer close(h.stopped)
	for {
		select {
		case <-h.ctx.Done():
			for {
				select {
				case req := <-h.publish:
					h.publishQueued(req)
				default:
					return
				}
			}
		case req := <-h.publish:
			h.publishQueued(req)
		}
	}
}

func (h *SSEHub) publishQueued(req publishRequest) {
	if err := h.Publish(context.Background(), req.weddingID, req.event); err != nil {
		log.Printf("SSE: publish %s for guest %s in wedding %s: %v", req.event.Type, req.event.GuestID, req.weddingID, err)
		// This replica could not publish, so no replica's clients can
		// trust their snapshot. Reconnect local clients and ask every
		// replica to do the same through Redis.
		h.reconnectWedding(req.weddingID)
		h.broadcastResync(context.Background(), req.weddingID)
	}
}

// Publish sends a guest event to Redis for cross-instance broadcasting.
func (h *SSEHub) Publish(ctx context.Context, weddingID uuid.UUID, event GuestEvent) error {
	select {
	case <-h.ready:
	default:
		return fmt.Errorf("SSE hub not ready")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal guest event: %w", err)
	}
	receivers, err := h.redis.Publish(ctx, fmt.Sprintf("wedding:%s:guests", weddingID.String()), data).Result()
	if err != nil {
		return err
	}
	// Our own pattern subscriber counts as a receiver. Zero means it is down or
	// reconnecting, and go-redis drops what was published in the gap, so local
	// clients reconnect and refetch instead of missing events silently.
	if receivers == 0 {
		h.reconnectWedding(weddingID)
	}
	return nil
}

// CloseStreams ends every local SSE stream so the HTTP server can drain. The
// hub keeps publishing; call Shutdown after the server has stopped.
func (h *SSEHub) CloseStreams() {
	h.mu.RLock()
	weddings := slices.Collect(maps.Keys(h.clients))
	h.mu.RUnlock()
	for _, wid := range weddings {
		h.reconnectWedding(wid)
	}
}

// Shutdown publishes the events still queued, bounded by ctx, then stops the
// Redis subscriber.
func (h *SSEHub) Shutdown(ctx context.Context) {
	h.cancel()
	select {
	case <-h.stopped:
	case <-ctx.Done():
		log.Printf("SSE: shutdown before the publish queue drained: %v", ctx.Err())
	}
	if h.pubsub != nil {
		if err := h.pubsub.Close(); err != nil {
			log.Printf("SSE: subscriber close failed: %v", err)
		}
	}
}

func extractWeddingID(channel string) uuid.UUID {
	parts := strings.Split(channel, ":")
	if len(parts) != 3 {
		return uuid.Nil
	}
	wid, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil
	}
	return wid
}
