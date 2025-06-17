package blackboard

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPutGetWatch(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	defer s.Close()

	store := NewRedisStore(&redis.Options{Addr: s.Addr()}, nil)
	ctx := context.Background()
	watch, err := store.Watch(ctx, "foo")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	ver, err := store.Put(ctx, "foo", "bar", time.Second)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	val, v, err := store.Get(ctx, "foo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val.(string) != "bar" || v != ver {
		t.Fatalf("unexpected value %v or version %d", val, v)
	}
	select {
	case <-watch:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for watch event")
	}
	store.Close()
}

func TestTxn(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	defer s.Close()

	store := NewRedisStore(&redis.Options{Addr: s.Addr()}, nil)
	ctx := context.Background()
	ops := map[string]interface{}{"a": 1, "b": 2}
	if err := store.Txn(ctx, ops, 0); err != nil {
		t.Fatalf("txn: %v", err)
	}
	val, _, err := store.Get(ctx, "a")
	if err != nil || val == nil {
		t.Fatalf("missing value after txn")
	}
	store.Close()
}
