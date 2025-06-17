package subsumption

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

// Layer represents a behavior layer with priority.
type Layer interface {
	Priority() int
	Name() string
	Check(ctx context.Context, bb blackboard.Store) bool
	Execute(ctx context.Context, bb blackboard.Store, bus eventbus.Bus) error
	Suppress() []int
}

// BaseLayer provides common functionality.
type BaseLayer struct {
	priority int
	name     string
	suppress []int
}

func (l BaseLayer) Priority() int   { return l.priority }
func (l BaseLayer) Name() string    { return l.name }
func (l BaseLayer) Suppress() []int { return l.suppress }

// AvoidanceLayer - highest priority emergency reactions
type AvoidanceLayer struct {
	BaseLayer
	dangerKey       string
	dangerThreshold float64
}

func NewAvoidanceLayer() *AvoidanceLayer {
	return &AvoidanceLayer{
		BaseLayer:       BaseLayer{priority: 100, name: "avoidance", suppress: []int{90, 50, 30, 10}},
		dangerKey:       "sensor.danger",
		dangerThreshold: 0.7,
	}
}

func (l *AvoidanceLayer) Check(ctx context.Context, bb blackboard.Store) bool {
	danger, _, err := bb.Get(ctx, l.dangerKey)
	if err != nil {
		return false
	}
	if d, ok := danger.(float64); ok {
		return d > l.dangerThreshold
	}
	return false
}

func (l *AvoidanceLayer) Execute(ctx context.Context, bb blackboard.Store, bus eventbus.Bus) error {
	bb.Put(ctx, "motor.command", "emergency_stop", 5*time.Second)
	bb.Put(ctx, "avoidance.active", true, 5*time.Second)

	event := core.Event{
		Type:      "subsumption.avoidance",
		Source:    l.name,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"action": "emergency_stop", "priority": l.priority},
	}
	return bus.Publish(ctx, "agent.action", event)
}

// WanderLayer - explore randomly when no goal
type WanderLayer struct {
	BaseLayer
	lastDirection string
	directions    []string
}

func NewWanderLayer() *WanderLayer {
	return &WanderLayer{
		BaseLayer:  BaseLayer{priority: 50, name: "wander", suppress: []int{30, 10}},
		directions: []string{"north", "south", "east", "west"},
	}
}

func (l *WanderLayer) Check(ctx context.Context, bb blackboard.Store) bool {
	goal, _, _ := bb.Get(ctx, "agent.goal")
	return goal == nil || goal == ""
}

func (l *WanderLayer) Execute(ctx context.Context, bb blackboard.Store, bus eventbus.Bus) error {
	dir := l.directions[time.Now().UnixNano()%int64(len(l.directions))]
	if dir == l.lastDirection && len(l.directions) > 1 {
		dir = l.directions[(time.Now().UnixNano()+1)%int64(len(l.directions))]
	}
	l.lastDirection = dir

	bb.Put(ctx, "motor.command", "move_"+dir, time.Second)
	return nil
}

// SeekGoalLayer - move toward goal
type SeekGoalLayer struct {
	BaseLayer
}

func NewSeekGoalLayer() *SeekGoalLayer {
	return &SeekGoalLayer{
		BaseLayer: BaseLayer{priority: 30, name: "seek_goal", suppress: []int{10}},
	}
}

func (l *SeekGoalLayer) Check(ctx context.Context, bb blackboard.Store) bool {
	goal, _, _ := bb.Get(ctx, "agent.goal")
	position, _, _ := bb.Get(ctx, "agent.position")
	return goal != nil && position != nil
}

func (l *SeekGoalLayer) Execute(ctx context.Context, bb blackboard.Store, bus eventbus.Bus) error {
	goal, _, _ := bb.Get(ctx, "agent.goal")
	position, _, _ := bb.Get(ctx, "agent.position")

	command := fmt.Sprintf("move_toward_%v", goal)
	bb.Put(ctx, "motor.command", command, time.Second)

	event := core.Event{
		Type:   "subsumption.seeking",
		Source: l.name,
		Payload: map[string]interface{}{
			"goal":     goal,
			"position": position,
		},
	}
	return bus.Publish(ctx, "agent.status", event)
}

// IdleLayer - default behavior
type IdleLayer struct {
	BaseLayer
}

func NewIdleLayer() *IdleLayer {
	return &IdleLayer{
		BaseLayer: BaseLayer{priority: 10, name: "idle"},
	}
}

func (l *IdleLayer) Check(ctx context.Context, bb blackboard.Store) bool { return true }

func (l *IdleLayer) Execute(ctx context.Context, bb blackboard.Store, bus eventbus.Bus) error {
	bb.Put(ctx, "motor.command", "stop", time.Second)
	bb.Put(ctx, "agent.status", "idle", time.Second)
	return nil
}

// SubsumptionAgent implements the subsumption architecture.
type SubsumptionAgent struct {
	id         string
	layers     []Layer
	eventBus   eventbus.Bus
	blackboard blackboard.Store

	tickInterval time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	logger       *log.Logger

	activeLayer string
	suppressed  map[string]bool

	wg sync.WaitGroup
}

// NewSubsumptionAgent creates a new subsumption agent.
func NewSubsumptionAgent(id string, bus eventbus.Bus, bb blackboard.Store, logger *log.Logger) *SubsumptionAgent {
	if logger == nil {
		logger = log.Default()
	}
	return &SubsumptionAgent{
		id:           id,
		eventBus:     bus,
		blackboard:   bb,
		tickInterval: 100 * time.Millisecond,
		logger:       logger,
		suppressed:   make(map[string]bool),
	}
}

func (a *SubsumptionAgent) AddLayer(layer Layer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.layers = append(a.layers, layer)
	sort.Slice(a.layers, func(i, j int) bool {
		return a.layers[i].Priority() > a.layers[j].Priority()
	})
}

func (a *SubsumptionAgent) ID() string { return a.id }

// Start begins the subsumption control loop.
func (a *SubsumptionAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.ctx != nil {
		a.mu.Unlock()
		return fmt.Errorf("agent already started")
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.mu.Unlock()

	sensorCh, err := a.eventBus.SubscribePattern(a.ctx, "sensor.*")
	if err != nil {
		return err
	}

	a.wg.Add(1)
	go a.run(sensorCh)
	return nil
}

func (a *SubsumptionAgent) run(sensorCh <-chan core.Event) {
	ticker := time.NewTicker(a.tickInterval)
	defer ticker.Stop()
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case ev := <-sensorCh:
			if ev.Type == "sensor.danger" {
				if val, ok := ev.Payload["value"]; ok {
					a.blackboard.Put(a.ctx, "sensor.danger", val, 10*time.Second)
				}
			}
		case <-ticker.C:
			a.arbitrate()
		}
	}
}

func (a *SubsumptionAgent) arbitrate() {
	a.mu.RLock()
	layers := a.layers
	a.mu.RUnlock()

	a.suppressed = make(map[string]bool)

	var active Layer
	var priority int
	for _, l := range layers {
		if l.Check(a.ctx, a.blackboard) {
			active = l
			priority = l.Priority()
			break
		}
	}

	if active == nil {
		return
	}

	for _, p := range active.Suppress() {
		for _, l := range layers {
			if l.Priority() <= p {
				a.suppressed[l.Name()] = true
			}
		}
	}

	if err := active.Execute(a.ctx, a.blackboard, a.eventBus); err != nil {
		a.logger.Printf("layer %s execute error: %v", active.Name(), err)
	}

	if a.activeLayer != active.Name() {
		a.activeLayer = active.Name()
		ev := core.Event{
			Type:   "subsumption.switch",
			Source: a.id,
			Payload: map[string]interface{}{
				"layer":      active.Name(),
				"priority":   priority,
				"suppressed": len(a.suppressed),
			},
		}
		a.eventBus.Publish(a.ctx, "agent.status", ev)
	}
}

func (a *SubsumptionAgent) Stop(ctx context.Context) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	a.mu.Lock()
	a.ctx = nil
	a.cancel = nil
	a.mu.Unlock()
	return nil
}

func (a *SubsumptionAgent) HandleEvent(event core.Event) error { return nil }

func (a *SubsumptionAgent) GetStatus() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]interface{}{
		"active_layer": a.activeLayer,
		"layers":       len(a.layers),
		"suppressed":   a.suppressed,
	}
}

var _ core.Agent = (*SubsumptionAgent)(nil)
