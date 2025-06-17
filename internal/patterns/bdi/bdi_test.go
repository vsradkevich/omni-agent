package bdi

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/eventbus"
)

func newTestAgent(t *testing.T) (*BDIAgent, context.Context, *blackboard.RedisStore) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("run miniredis: %v", err)
	}
	bus := eventbus.NewRedisBus(&redis.Options{Addr: mr.Addr()}, nil)
	store := blackboard.NewRedisStore(&redis.Options{Addr: mr.Addr()}, nil)
	ctx := context.Background()
	ag := NewBDIAgent("a1", bus, store, nil)
	return ag, ctx, store
}

func TestSelectGoal(t *testing.T) {
	ag, ctx, _ := newTestAgent(t)
	ag.AddBelief("foo", "bar", 1)
	goal := Goal{ID: "g1", Priority: 1, Condition: func(b map[string]Belief) bool {
		v, ok := b["foo"]
		return ok && v.Value == "bar"
	}}
	ag.AddGoal(goal)
	if g := ag.SelectGoal(); g == nil || g.ID != "g1" {
		t.Fatalf("expected g1 selected, got %v", g)
	}
	ag.Stop(ctx)
}

func TestExecutePlan(t *testing.T) {
	ag, ctx, store := newTestAgent(t)
	goal := Goal{ID: "g2", Priority: 1}
	plan, err := ag.GeneratePlan(&goal)
	if err != nil {
		t.Fatalf("generate plan: %v", err)
	}
	ag.intention = plan
	if err := ag.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	// execute step manually for deterministic test
	if err := ag.ExecutePlan(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	val, _, err := store.Get(ctx, "g2")
	if err != nil || val.(string) != "done" {
		t.Fatalf("expected done value, got %v err %v", val, err)
	}
	ag.Stop(ctx)
}
