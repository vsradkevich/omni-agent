package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go-agent-framework/internal/core"
)

func TestPublishSubscribe(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	defer s.Close()

	bus := NewRedisBus(&redis.Options{Addr: s.Addr()}, nil)
	ctx := context.Background()
	ch, err := bus.Subscribe(ctx, "agent.test")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	ev := core.Event{ID: "1", Type: "ping", Timestamp: time.Now()}
	if err := bus.Publish(ctx, "agent.test", ev); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case got := <-ch:
		if got.ID != ev.ID {
			t.Fatalf("expected %s got %s", ev.ID, got.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
	bus.Close()
}

func TestPatternSubscribe(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	defer s.Close()

	bus := NewRedisBus(&redis.Options{Addr: s.Addr()}, nil)
	ctx := context.Background()
	ch, err := bus.SubscribePattern(ctx, "agent.*")
	if err != nil {
		t.Fatalf("subscribe pattern: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	ev := core.Event{ID: "2", Type: "ping", Timestamp: time.Now()}
	if err := bus.Publish(ctx, "agent.42", ev); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for pattern event")
	}
	bus.Close()
}
