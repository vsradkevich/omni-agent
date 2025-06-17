package bdi

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

// Belief represents a piece of knowledge held by the agent.
type Belief struct {
	Key        string
	Value      interface{}
	Confidence float64
	Timestamp  time.Time
}

// Goal describes a desired world state.
type Goal struct {
	ID          string
	Description string
	Priority    int
	Achieved    bool
	Condition   func(map[string]Belief) bool
}

// Plan defines a sequence of actions to achieve a goal.
type Plan struct {
	ID          string
	GoalID      string
	Steps       []Action
	CurrentStep int
	Status      core.Status
}

// Action represents an executable step in a plan.
type Action interface {
	Execute(ctx context.Context, agent *BDIAgent) error
	String() string
}

// WriteBlackboardAction stores a value on the blackboard.
type WriteBlackboardAction struct {
	Key   string
	Value interface{}
}

func (a WriteBlackboardAction) Execute(ctx context.Context, agent *BDIAgent) error {
	_, err := agent.blackboard.Put(ctx, a.Key, a.Value, 0)
	return err
}

func (a WriteBlackboardAction) String() string { return "write:" + a.Key }

// PublishEventAction publishes an event on the bus.
type PublishEventAction struct {
	Topic string
	Event core.Event
}

func (a PublishEventAction) Execute(ctx context.Context, agent *BDIAgent) error {
	return agent.eventBus.Publish(ctx, a.Topic, a.Event)
}

func (a PublishEventAction) String() string { return "publish:" + a.Topic }

// WaitAction pauses for a duration.
type WaitAction struct{ Duration time.Duration }

func (a WaitAction) Execute(ctx context.Context, agent *BDIAgent) error {
	select {
	case <-time.After(a.Duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a WaitAction) String() string { return "wait" }

// BDIAgent implements the Belief-Desire-Intention model.
type BDIAgent struct {
	id         string
	beliefs    sync.Map // key -> Belief
	desires    []Goal
	intention  *Plan
	eventBus   eventbus.Bus
	blackboard blackboard.Store

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	logger *log.Logger
}

// NewBDIAgent creates a new BDIAgent instance.
func NewBDIAgent(id string, bus eventbus.Bus, bb blackboard.Store, logger *log.Logger) *BDIAgent {
	if logger == nil {
		logger = log.Default()
	}
	return &BDIAgent{id: id, eventBus: bus, blackboard: bb, logger: logger}
}

// ID returns the agent identifier.
func (a *BDIAgent) ID() string { return a.id }

// Start launches the agent reasoning loop.
func (a *BDIAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.ctx != nil {
		a.mu.Unlock()
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.mu.Unlock()

	go a.run()
	return nil
}

// Stop terminates the agent.
func (a *BDIAgent) Stop(ctx context.Context) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.ctx = nil
	a.cancel = nil
	a.mu.Unlock()
	return nil
}

func (a *BDIAgent) run() {
	busCh, err := a.eventBus.SubscribePattern(a.ctx, "bdi.belief.*")
	if err != nil {
		a.logger.Println("subscribe error", err)
	}
	defer func() {
		if err := a.eventBus.Unsubscribe(a.ctx, "bdi.belief.*"); err != nil {
			a.logger.Println("unsubscribe error", err)
		}
	}()
	bbCh, err := a.blackboard.Watch(a.ctx, "*")
	if err != nil {
		a.logger.Println("blackboard watch error", err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case ev, ok := <-busCh:
			if ok {
				_ = a.HandleEvent(ev)
			}
		case upd, ok := <-bbCh:
			if ok {
				a.AddBelief(upd.Key, upd.Value, 1.0)
			}
		case <-ticker.C:
			a.reason()
		}
	}
}

// HandleEvent updates beliefs based on an incoming event.
func (a *BDIAgent) HandleEvent(ev core.Event) error {
	if ev.Type == "belief" {
		if key, ok := ev.Payload["key"].(string); ok {
			a.AddBelief(key, ev.Payload["value"], 1.0)
		}
	}
	return nil
}

// AddBelief inserts or updates a belief.
func (a *BDIAgent) AddBelief(key string, value interface{}, conf float64) {
	b := Belief{Key: key, Value: value, Confidence: conf, Timestamp: time.Now()}
	a.beliefs.Store(key, b)
}

// GetBelief returns a belief if present.
func (a *BDIAgent) GetBelief(key string) (Belief, bool) {
	v, ok := a.beliefs.Load(key)
	if !ok {
		return Belief{}, false
	}
	return v.(Belief), true
}

// GetAllBeliefs returns a snapshot of the current belief map.
func (a *BDIAgent) GetAllBeliefs() map[string]Belief {
	beliefs := make(map[string]Belief)
	a.beliefs.Range(func(k, v interface{}) bool {
		beliefs[k.(string)] = v.(Belief)
		return true
	})
	return beliefs
}

// UpdateBeliefFromBlackboard fetches a key from the blackboard and stores it.
func (a *BDIAgent) UpdateBeliefFromBlackboard(key string) {
	val, _, err := a.blackboard.Get(a.ctx, key)
	if err != nil {
		a.logger.Println("blackboard get error", err)
		return
	}
	a.AddBelief(key, val, 1.0)
}

// AddGoal adds a new desire.
func (a *BDIAgent) AddGoal(goal Goal) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.desires = append(a.desires, goal)
}

// SelectGoal chooses the highest priority achievable goal.
func (a *BDIAgent) SelectGoal() *Goal {
	a.mu.RLock()
	defer a.mu.RUnlock()
	beliefs := a.GetAllBeliefs()
	sort.Slice(a.desires, func(i, j int) bool { return a.desires[i].Priority > a.desires[j].Priority })
	for i := range a.desires {
		g := &a.desires[i]
		if g.Achieved {
			continue
		}
		if g.Condition == nil || g.Condition(beliefs) {
			return g
		}
	}
	return nil
}

// GeneratePlan creates a simple plan for a goal.
func (a *BDIAgent) GeneratePlan(goal *Goal) (*Plan, error) {
	plan := &Plan{
		ID:          "plan-" + goal.ID,
		GoalID:      goal.ID,
		Steps:       []Action{WriteBlackboardAction{Key: goal.ID, Value: "done"}},
		CurrentStep: 0,
		Status:      core.StatusRunning,
	}
	return plan, nil
}

// ExecutePlan performs the next step of the current intention.
func (a *BDIAgent) ExecutePlan() error {
	a.mu.Lock()
	plan := a.intention
	a.mu.Unlock()
	if plan == nil || plan.Status != core.StatusRunning {
		return nil
	}

	if plan.CurrentStep >= len(plan.Steps) {
		plan.Status = core.StatusSuccess
		a.markGoalAchieved(plan.GoalID)
		return nil
	}

	step := plan.Steps[plan.CurrentStep]
	if err := step.Execute(a.ctx, a); err != nil {
		a.logger.Println("step error", err)
		plan.Status = core.StatusFailure
		return err
	}
	plan.CurrentStep++
	return nil
}

func (a *BDIAgent) markGoalAchieved(goalID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.desires {
		if a.desires[i].ID == goalID {
			a.desires[i].Achieved = true
		}
	}
}

func (a *BDIAgent) reason() {
	goal := a.SelectGoal()
	if goal == nil {
		return
	}
	a.mu.Lock()
	if a.intention == nil || a.intention.GoalID != goal.ID || a.intention.Status != core.StatusRunning {
		plan, err := a.GeneratePlan(goal)
		if err != nil {
			a.logger.Println("plan generation error", err)
			a.mu.Unlock()
			return
		}
		a.intention = plan
	}
	a.mu.Unlock()
	_ = a.ExecutePlan()
}
