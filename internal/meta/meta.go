package meta

import (
	"context"
	"fmt"
	"sync"

	"go-agent-framework/internal/core"
)

// AgentFactory creates agents of various types.
type AgentFactory interface {
	Create(id, kind string) (core.Agent, error)
}

// MetaAgent manages the lifecycle of other agents.
type MetaAgent struct {
	id       string
	registry map[string]core.Agent
	factory  AgentFactory
	mu       sync.RWMutex
}

// NewMetaAgent returns a new MetaAgent instance.
func NewMetaAgent(id string, f AgentFactory) *MetaAgent {
	return &MetaAgent{id: id, registry: make(map[string]core.Agent), factory: f}
}

// ID returns the identifier of the meta agent.
func (m *MetaAgent) ID() string { return m.id }

// Start implements core.Agent but performs no action.
func (m *MetaAgent) Start(ctx context.Context) error { return nil }

// Stop stops all managed agents.
func (m *MetaAgent) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ag := range m.registry {
		ag.Stop(ctx)
	}
	return nil
}

// HandleEvent processes events but currently does nothing.
func (m *MetaAgent) HandleEvent(ev core.Event) error { return nil }

// SpawnAgent creates and starts a new agent.
func (m *MetaAgent) SpawnAgent(ctx context.Context, id, kind string) error {
	ag, err := m.factory.Create(id, kind)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	if err := ag.Start(ctx); err != nil {
		return fmt.Errorf("start agent: %w", err)
	}
	m.mu.Lock()
	m.registry[id] = ag
	m.mu.Unlock()
	return nil
}

// AgentIDs returns the list of managed agent identifiers.
func (m *MetaAgent) AgentIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.registry))
	for id := range m.registry {
		ids = append(ids, id)
	}
	return ids
}

var _ core.Agent = (*MetaAgent)(nil)
