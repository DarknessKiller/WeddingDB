package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"weddingdb/internal/testutil"
)

func TestSSEHub_DistributedWeddingLimit(t *testing.T) {
	addr := testutil.RedisAddr(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	testutil.NoErr(t, rdb.Ping(context.Background()).Err())
	hubs := []*SSEHub{NewSSEHub(rdb), NewSSEHub(rdb)}
	for _, hub := range hubs {
		t.Cleanup(hub.Shutdown)
	}
	wid := uuid.New()
	clients := make([]*SSEClient, 200)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() { clients[i] = testSubscribe(t, hubs[i%2], wid) })
	}
	wg.Wait()
	count := 0
	for i, client := range clients {
		if client != nil {
			count++
			t.Cleanup(func() { hubs[i%2].Unsubscribe(client) })
		}
	}
	if count != 100 {
		t.Fatalf("accepted %d streams across hubs, want 100", count)
	}
	other, err := hubs[1].Subscribe(context.Background(), uuid.New(), uuid.New())
	testutil.NoErr(t, err)
	t.Cleanup(func() { hubs[1].Unsubscribe(other) })
	var released *SSEClient
	for i, client := range clients {
		if client != nil {
			hubs[i%2].Unsubscribe(client)
			released = client
			break
		}
	}
	replacement, err := hubs[1].Subscribe(context.Background(), wid, uuid.New())
	testutil.NoErr(t, err)
	t.Cleanup(func() { hubs[1].Unsubscribe(replacement) })
	testutil.NoErr(t, rdb.ZAdd(context.Background(), replacement.key(), redis.Z{Score: 0, Member: replacement.ID}).Err())
	reclaimed, err := hubs[0].Subscribe(context.Background(), wid, uuid.New())
	testutil.NoErr(t, err)
	t.Cleanup(func() { hubs[0].Unsubscribe(reclaimed) })
	select {
	case <-replacement.Done:
	case <-time.After(12 * time.Second):
		t.Fatal("expired lease did not close stream")
	}
	if released == nil {
		t.Fatal("no stream released")
	}
}

func TestSSEHub_RenewalFailureClosesStream(t *testing.T) {
	addr := testutil.RedisAddr(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	hub := NewSSEHub(rdb)
	t.Cleanup(hub.Shutdown)
	client, err := hub.Subscribe(context.Background(), uuid.New(), uuid.New())
	testutil.NoErr(t, err)
	testutil.NoErr(t, rdb.Close())
	select {
	case <-client.Done:
	case <-time.After(12 * time.Second):
		t.Fatal("Redis failure did not close stream")
	}
	if _, err := hub.Subscribe(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("Redis failure admitted a stream")
	}
}
