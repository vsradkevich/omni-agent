package subsumption

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

func TestSubsumptionLayerPriority(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	bus := eventbus.NewRedisBus(&redis.Options{Addr: mr.Addr()}, nil)
	bb := blackboard.NewRedisStore(&redis.Options{Addr: mr.Addr()}, nil)

	ag := NewSubsumptionAgent("test", bus, bb, nil)
	ag.tickInterval = 10 * time.Millisecond

	ag.AddLayer(NewIdleLayer())
	ag.AddLayer(NewAvoidanceLayer())
	ag.AddLayer(NewWanderLayer())
	ag.AddLayer(NewSeekGoalLayer())

	ctx := context.Background()

	statusCh, _ := bus.Subscribe(ctx, "agent.status")

	ag.Start(ctx)
	defer ag.Stop(ctx)
	// drain initial status event
	select {
	case <-statusCh:
	case <-time.After(20 * time.Millisecond):
	}

	time.Sleep(20 * time.Millisecond)
	st := ag.GetStatus()
	// Depending on timing the wander layer may be suppressed, so allow idle
	if st["active_layer"] != "wander" && st["active_layer"] != "idle" {
		t.Errorf("expected wander or idle layer, got %v", st["active_layer"])
	}

	bb.Put(ctx, "sensor.danger", 0.9, time.Second)
	dangerEvent := core.Event{Type: "sensor.danger", Source: "test", Payload: map[string]interface{}{"value": 0.9}}
	bus.Publish(ctx, "sensor.danger", dangerEvent)
	time.Sleep(20 * time.Millisecond)

	select {
	case ev := <-statusCh:
		if ev.Type == "subsumption.switch" {
			if layer, ok := ev.Payload["layer"].(string); ok && layer != "avoidance" {
				t.Errorf("expected avoidance layer, got %s", layer)
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for layer switch")
	}

	st = ag.GetStatus()
	suppressed := st["suppressed"].(map[string]bool)
	if !suppressed["wander"] || !suppressed["seek_goal"] || !suppressed["idle"] {
		t.Error("lower priority layers should be suppressed")
	}

	ag.Stop(ctx)
	time.Sleep(20 * time.Millisecond)
	bus.Close()
	bb.Close()
}

func TestSubsumptionGoalSeeking(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	bus := eventbus.NewRedisBus(&redis.Options{Addr: mr.Addr()}, nil)
	bb := blackboard.NewRedisStore(&redis.Options{Addr: mr.Addr()}, nil)

	ag := NewSubsumptionAgent("seeker", bus, bb, nil)
	ag.tickInterval = 10 * time.Millisecond
	ag.AddLayer(NewIdleLayer())
	ag.AddLayer(NewSeekGoalLayer())
	ag.AddLayer(NewWanderLayer())

	ctx := context.Background()
	ag.Start(ctx)
	defer ag.Stop(ctx)

	bb.Put(ctx, "agent.goal", "target_location", time.Second)
	bb.Put(ctx, "agent.position", "start_location", time.Second)

	time.Sleep(50 * time.Millisecond)

	st := ag.GetStatus()
	if st["active_layer"] != "seek_goal" {
		t.Errorf("expected seek_goal layer, got %v", st["active_layer"])
	}

	cmd, _, _ := bb.Get(ctx, "motor.command")
	if cmd == nil || cmd.(string) == "stop" {
		t.Error("expected movement command")
	}

	ag.Stop(ctx)
	time.Sleep(20 * time.Millisecond)
	bus.Close()
	bb.Close()
}
