package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
}

// maxClientsPerWedding bounds the fan-out and memory one wedding's stream can
// hold on a single instance.
const maxClientsPerWedding = 100

// maxClientsPerUser stops one account from taking every slot in a wedding.
const maxClientsPerUser = 5

// SSEClient represents a connected SSE client.
type SSEClient struct {
	ID        string
	WeddingID uuid.UUID
	OwnerID   uuid.UUID
	Chan      chan []byte
	Done      chan struct{}
	doneOnce  sync.Once
}

// SSEHub manages SSE connections per wedding and Redis pub/sub.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[string]*SSEClient // weddingID -> clientID -> client
	redis   *redis.Client
	pubsub  *redis.PubSub
	ctx     context.Context
	cancel  context.CancelFunc
	// ponytail: closed after PSubscribe ACKs so the first Publish is not lost during Redis warm-up.
	// Race B (client joins SSE mid-publish) is not addressed here — fix needs a per-wedding replay buffer.
	ready chan struct{}
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
	h.pubsub = h.redis.PSubscribe(h.ctx, "wedding:*:guests")
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
	}
}

// Subscribe registers a new SSE client for a wedding. It reports false when the
// per-wedding or per-user connection cap is reached.
func (h *SSEHub) Subscribe(weddingID, ownerID uuid.UUID) (*SSEClient, bool) {
	h.mu.Lock()
	clients := h.clients[weddingID]
	if len(clients) >= maxClientsPerWedding {
		h.mu.Unlock()
		return nil, false
	}
	sameOwner := 0
	for _, existing := range clients {
		if existing.OwnerID == ownerID {
			sameOwner++
		}
	}
	if sameOwner >= maxClientsPerUser {
		h.mu.Unlock()
		return nil, false
	}
	client := &SSEClient{ID: uuid.New().String(), WeddingID: weddingID, OwnerID: ownerID, Chan: make(chan []byte, 64), Done: make(chan struct{})}
	if clients == nil {
		clients = make(map[string]*SSEClient)
		h.clients[weddingID] = clients
	}
	clients[client.ID] = client
	h.mu.Unlock()
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

// Shutdown gracefully stops the Redis subscriber.
func (h *SSEHub) Shutdown() {
	h.cancel()
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
