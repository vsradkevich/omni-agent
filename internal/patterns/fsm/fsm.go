package fsm

import (
	"context"
	"fmt"
	"log"
	"sync"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

// State represents a state identifier.
type State string

// Event represents a transition trigger.
type Event string

// Transition defines a state change caused by an event.
type Transition struct {
	From   State
	Event  Event
	To     State
	Action func(ctx context.Context) error
}

// StateActions groups callbacks for a state lifecycle.
type StateActions struct {
	OnEnter func(ctx context.Context) error
	OnExit  func(ctx context.Context) error
	OnStay  func(ctx context.Context) error
}

// FSM is a simple finite state machine implementation.
type FSM struct {
	id           string
	currentState State
	transitions  map[State]map[Event]Transition
	stateActions map[State]StateActions
	eventBus     eventbus.Bus
	blackboard   blackboard.Store
	mu           sync.RWMutex
	logger       *log.Logger
}

// NewFSM creates a new FSM.
func NewFSM(id string, initialState State, bus eventbus.Bus, bb blackboard.Store, logger *log.Logger) *FSM {
	if logger == nil {
		logger = log.Default()
	}
	return &FSM{
		id:           id,
		currentState: initialState,
		transitions:  make(map[State]map[Event]Transition),
		stateActions: make(map[State]StateActions),
		eventBus:     bus,
		blackboard:   bb,
		logger:       logger,
	}
}

// ID returns the FSM identifier.
func (f *FSM) ID() string { return f.id }

// Start implements the Agent interface. No-op for now.
func (f *FSM) Start(ctx context.Context) error { return nil }

// Stop implements the Agent interface. No-op for now.
func (f *FSM) Stop(ctx context.Context) error { return nil }

// HandleEvent allows integration with other agents.
// HandleEvent triggers transitions via event payloads.
func (f *FSM) HandleEvent(ev core.Event) error {
	if ev.Type == "fsm.trigger" {
		if name, ok := ev.Payload["event"].(string); ok {
			return f.Trigger(context.Background(), Event(name))
		}
	}
	return nil
}

// AddTransition registers a transition.
func (f *FSM) AddTransition(t Transition) {
	if _, ok := f.transitions[t.From]; !ok {
		f.transitions[t.From] = make(map[Event]Transition)
	}
	f.transitions[t.From][t.Event] = t
}

// AddStateActions sets callbacks for a state.
func (f *FSM) AddStateActions(s State, actions StateActions) { f.stateActions[s] = actions }

// ValidateTransitions checks that all states are reachable from the initial state.
func (f *FSM) ValidateTransitions() error {
	reachable := map[State]bool{f.currentState: true}
	for from, evs := range f.transitions {
		for _, t := range evs {
			if from == "" || t.To == "" {
				return fmt.Errorf("invalid transition %v", t)
			}
			reachable[t.To] = true
		}
		reachable[from] = true
	}
	for s := range f.stateActions {
		if !reachable[s] {
			return fmt.Errorf("state %s unreachable", s)
		}
	}
	return nil
}

// Trigger moves the FSM according to an event.
func (f *FSM) Trigger(ctx context.Context, e Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	trans, ok := f.transitions[f.currentState][e]
	if !ok {
		return nil
	}
	if act, ok := f.stateActions[f.currentState]; ok && act.OnExit != nil {
		_ = act.OnExit(ctx)
	}
	if trans.Action != nil {
		if err := trans.Action(ctx); err != nil {
			return err
		}
	}
	f.currentState = trans.To
	if act, ok := f.stateActions[f.currentState]; ok && act.OnEnter != nil {
		_ = act.OnEnter(ctx)
	}
	return nil
}

// GetState returns the current state.
func (f *FSM) GetState() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.currentState
}
