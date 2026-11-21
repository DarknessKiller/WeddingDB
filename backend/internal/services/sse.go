package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
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
	Pinyin      string   `json:"pinyin"`
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
}

// maxClientsPerWedding bounds the fan-out and memory one wedding's stream can
// hold on a single instance.
const maxClientsPerWedding = 100

// maxClientsPerUser stops one account from taking every slot in a wedding.
// It is sized for one shared door login on every phone and tablet at the
// reception: below the device count, each reconnect evicts another device and
// the streams evict each other forever.
const maxClientsPerUser = 20

// SSEClient represents a connected SSE client.
type SSEClient struct {
	ID        string
	WeddingID uuid.UUID
	OwnerID   uuid.UUID
	Chan      chan []byte
	Done      chan struct{}
	doneOnce  sync.Once
	openedAt  time.Time
}

// SSEHub manages SSE connections per wedding and Redis pub/sub.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[string]*SSEClient // weddingID -> clientID -> client
	redis   *redis.Client
	ctx     context.Context
	cancel  context.CancelFunc
	// ponytail: closed after PSubscribe ACKs so the first Publish is not lost during Redis warm-up.
	// Race B (client joins SSE mid-publish) is not addressed here — fix needs a per-wedding replay buffer.
	ready chan struct{}
	// pausedUntil (unix nanos) skips publishes for a while after one fails,
	// so a stalled Redis costs one publishTimeout per pause, not one per row
	// of an import or sync batch.
	pausedUntil atomic.Int64
}

// NewSSEHub creates a new SSEHub and starts the Redis subscriber.
func NewSSEHub(rdb *redis.Client) *SSEHub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &SSEHub{clients: make(map[uuid.UUID]map[string]*SSEClient), redis: rdb, ctx: ctx, cancel: cancel, ready: make(chan struct{})}
	go hub.subscribeRedis()
	select {
	case <-hub.ready:
	case <-time.After(5 * time.Second):
		log.Printf("SSE: PSubscribe not ready after 5s; Publish will error until Redis recovers")
	}
	return hub
}

// subscribeRedis listens on the wildcard Redis channel and fans out to local clients.
func (h *SSEHub) subscribeRedis() {
	// The subscription lives only in this goroutine, which closes it on exit;
	// Shutdown may run before PSubscribe returns (Redis slow to dial).
	pubsub := h.redis.PSubscribe(h.ctx, "wedding:*:guests")
	defer func() {
		if err := pubsub.Close(); err != nil {
			log.Printf("SSE: subscriber close failed: %v", err)
		}
	}()
	// Wait for Redis to confirm the subscription is active
	if err := pubsub.Ping(h.ctx); err != nil {
		log.Printf("SSE: PSubscribe ping failed: %v", err)
	}
	close(h.ready)
	// With subscriptions: the first *redis.Subscription confirms PSubscribe;
	// any later one is go-redis re-subscribing after a dropped connection.
	// Events published in that gap are gone, so every local stream resyncs.
	ch := pubsub.ChannelWithSubscriptions()
	subscribed := false
	for {
		select {
		case <-h.ctx.Done():
			return
		case raw, ok := <-ch:
			if !ok {
				return
			}
			switch msg := raw.(type) {
			case *redis.Subscription:
				if subscribed {
					h.resync(uuid.Nil)
				}
				subscribed = true
			case *redis.Message:
				h.fanOut(msg)
			}
		}
	}
}

func (h *SSEHub) fanOut(msg *redis.Message) {
	wid := extractWeddingID(msg.Channel)
	if wid == uuid.Nil {
		return
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
			h.Unsubscribe(client)
		}
	}
}

// resync ends every local stream of weddingID (of every wedding for
// uuid.Nil) after an event for it was lost. Pings keep a stream alive, so
// without this a client never reconnects and reloads the guests it missed.
func (h *SSEHub) resync(weddingID uuid.UUID) {
	if h == nil {
		return
	}
	var ended []*SSEClient
	h.mu.Lock()
	for wid, clients := range h.clients {
		if weddingID != uuid.Nil && wid != weddingID {
			continue
		}
		for _, c := range clients {
			ended = append(ended, c)
		}
		delete(h.clients, wid)
	}
	h.mu.Unlock()
	for _, c := range ended {
		c.doneOnce.Do(func() { close(c.Done) })
	}
	if len(ended) > 0 {
		log.Printf("SSE: ended %d streams to resync after lost events", len(ended))
	}
}

// Subscribe registers a new SSE client for a wedding. It reports false when the
// per-wedding cap is reached; at the per-user cap it evicts that user's oldest stream.
func (h *SSEHub) Subscribe(weddingID, ownerID uuid.UUID) (*SSEClient, bool) {
	h.mu.Lock()
	clients := h.clients[weddingID]
	if len(clients) >= maxClientsPerWedding {
		h.mu.Unlock()
		return nil, false
	}
	// At the per-user cap, evict that owner's oldest stream: a phone that went
	// away without closing its socket would otherwise hold a slot for minutes.
	sameOwner := 0
	var oldest *SSEClient
	for _, existing := range clients {
		if existing.OwnerID == ownerID {
			sameOwner++
			if oldest == nil || existing.openedAt.Before(oldest.openedAt) {
				oldest = existing
			}
		}
	}
	var evicted *SSEClient
	if sameOwner >= maxClientsPerUser {
		delete(clients, oldest.ID)
		evicted = oldest
	}
	client := &SSEClient{ID: uuid.New().String(), WeddingID: weddingID, OwnerID: ownerID, Chan: make(chan []byte, 64), Done: make(chan struct{}), openedAt: time.Now()}
	if clients == nil {
		clients = make(map[string]*SSEClient)
		h.clients[weddingID] = clients
	}
	clients[client.ID] = client
	h.mu.Unlock()
	if evicted != nil {
		evicted.doneOnce.Do(func() { close(evicted.Done) })
		log.Printf("SSE: client %s evicted from wedding %s (per-user cap)", evicted.ID, weddingID)
	}
	log.Printf("SSE: client %s connected for wedding %s", client.ID, weddingID)
	return client, true
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
	log.Printf("SSE: client %s disconnected from wedding %s", client.ID, client.WeddingID)
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
	return h.redis.Publish(ctx, fmt.Sprintf("wedding:%s:guests", weddingID.String()), data).Err()
}

// Shutdown stops the Redis subscriber and ends every open stream.
func (h *SSEHub) Shutdown() {
	h.cancel()
	// End every open stream so a graceful HTTP shutdown is not held open by
	// connections that would otherwise last until the client leaves.
	h.mu.Lock()
	for _, clients := range h.clients {
		for _, c := range clients {
			c.doneOnce.Do(func() { close(c.Done) })
		}
	}
	h.mu.Unlock()
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
