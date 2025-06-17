package meta

import (
	"context"
	"testing"

	"go-agent-framework/internal/core"
)

type stubFactory struct{ a core.Agent }

func (f stubFactory) Create(id, kind string) (core.Agent, error) { return f.a, nil }

type dummyAgent struct{ started bool }

func (d *dummyAgent) ID() string                      { return "dummy" }
func (d *dummyAgent) Start(ctx context.Context) error { d.started = true; return nil }
func (d *dummyAgent) Stop(ctx context.Context) error  { d.started = false; return nil }
func (d *dummyAgent) HandleEvent(ev core.Event) error { return nil }

func TestSpawnAgent(t *testing.T) {
	da := &dummyAgent{}
	m := NewMetaAgent("meta1", stubFactory{da})
	ctx := context.Background()
	if err := m.SpawnAgent(ctx, "a1", "dummy"); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !da.started {
		t.Fatal("agent should be started")
	}
	if len(m.AgentIDs()) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(m.AgentIDs()))
	}
	m.Stop(ctx)
	if da.started {
		t.Fatal("agent should be stopped")
	}
}
