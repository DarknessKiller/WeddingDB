package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

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
	Phone       string   `json:"phone"`
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

// SSEClient represents a connected SSE client.
type SSEClient struct {
	ID        string
	WeddingID uuid.UUID
	Chan      chan []byte
	Done      chan struct{}
}

// SSEHub manages SSE connections per wedding and Redis pub/sub.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[string]*SSEClient // weddingID -> clientID -> client
	redis   *redis.Client
	pubsub  *redis.PubSub
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewSSEHub creates a new SSEHub and starts the Redis subscriber.
func NewSSEHub(rdb *redis.Client) *SSEHub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &SSEHub{
		clients: make(map[uuid.UUID]map[string]*SSEClient),
		redis:   rdb,
		ctx:     ctx,
		cancel:  cancel,
	}
	go hub.subscribeRedis()
	return hub
}

// subscribeRedis listens on the wildcard Redis channel and fans out to local clients.
func (h *SSEHub) subscribeRedis() {
	h.pubsub = h.redis.Subscribe(h.ctx, "wedding:*:guests")

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
			clients := h.clients[wid]
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.Chan <- []byte(msg.Payload):
				default:
					// Client buffer full, drop event to avoid blocking.
				}
			}
		}
	}
}

// Subscribe registers a new SSE client for a wedding.
func (h *SSEHub) Subscribe(weddingID uuid.UUID) *SSEClient {
	client := &SSEClient{
		ID:        uuid.New().String(),
		WeddingID: weddingID,
		Chan:      make(chan []byte, 64),
		Done:      make(chan struct{}),
	}

	h.mu.Lock()
	if h.clients[weddingID] == nil {
		h.clients[weddingID] = make(map[string]*SSEClient)
	}
	h.clients[weddingID][client.ID] = client
	h.mu.Unlock()

	log.Printf("SSE: client %s connected for wedding %s", client.ID, weddingID)
	return client
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
	close(client.Done)
	log.Printf("SSE: client %s disconnected from wedding %s", client.ID, client.WeddingID)
}

// Publish sends a guest event to Redis for cross-instance broadcasting.
// The provided ctx is propagated to Redis for cancellation/deadline support.
func (h *SSEHub) Publish(ctx context.Context, weddingID uuid.UUID, event GuestEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal guest event: %w", err)
	}

	channel := fmt.Sprintf("wedding:%s:guests", weddingID.String())
	return h.redis.Publish(ctx, channel, data).Err()
}

// Shutdown gracefully stops the Redis subscriber.
func (h *SSEHub) Shutdown() {
	h.cancel()
	if h.pubsub != nil {
		h.pubsub.Close()
	}
}

func extractWeddingID(channel string) uuid.UUID {
	// Channel format: "wedding:{wid}:guests"
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
