package bt

import (
	"context"
	"log"
	"time"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

// Status mirrors core.Status for simplicity.
type Status = core.Status

// Node defines the behavior tree node interface.
type Node interface {
	Tick(ctx context.Context, bb blackboard.Store) Status
	Reset()
}

// Composite nodes ------------------------------------------------------------

// SequenceNode executes children sequentially until one fails.
type SequenceNode struct {
	children []Node
	current  int
}

func (n *SequenceNode) AddChild(c Node) { n.children = append(n.children, c) }

func (n *SequenceNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	for n.current < len(n.children) {
		st := n.children[n.current].Tick(ctx, bb)
		if st == core.StatusRunning {
			return core.StatusRunning
		}
		if st == core.StatusFailure {
			n.current = 0
			return core.StatusFailure
		}
		n.current++
	}
	n.current = 0
	return core.StatusSuccess
}

func (n *SequenceNode) Reset() {
	n.current = 0
	for _, c := range n.children {
		c.Reset()
	}
}

// SelectorNode executes children until one succeeds.
type SelectorNode struct {
	children []Node
}

func (n *SelectorNode) AddChild(c Node) { n.children = append(n.children, c) }

func (n *SelectorNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	for _, child := range n.children {
		st := child.Tick(ctx, bb)
		if st == core.StatusSuccess {
			return core.StatusSuccess
		}
		if st == core.StatusRunning {
			return core.StatusRunning
		}
	}
	return core.StatusFailure
}

func (n *SelectorNode) Reset() {
	for _, c := range n.children {
		c.Reset()
	}
}

// Leaf nodes -------------------------------------------------------------

type ActionNode struct {
	action func(context.Context, blackboard.Store) Status
}

func NewActionNode(f func(context.Context, blackboard.Store) Status) *ActionNode {
	return &ActionNode{action: f}
}

func (n *ActionNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	return n.action(ctx, bb)
}

func (n *ActionNode) Reset() {}

type ConditionNode struct {
	condition func(blackboard.Store) bool
}

func NewConditionNode(f func(blackboard.Store) bool) *ConditionNode {
	return &ConditionNode{condition: f}
}

func (n *ConditionNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	if n.condition(bb) {
		return core.StatusSuccess
	}
	return core.StatusFailure
}

func (n *ConditionNode) Reset() {}

// Helpers for building nodes --------------------------------------------------

func NewSequence(children ...Node) *SequenceNode {
	n := &SequenceNode{}
	for _, c := range children {
		n.AddChild(c)
	}
	return n
}

func NewSelector(children ...Node) *SelectorNode {
	n := &SelectorNode{}
	for _, c := range children {
		n.AddChild(c)
	}
	return n
}

func NewParallel(policy ParallelPolicy, children ...Node) *ParallelNode {
	n := &ParallelNode{policy: policy}
	for _, c := range children {
		n.AddChild(c)
	}
	return n
}

// Additional nodes -----------------------------------------------------------

// ParallelPolicy defines success conditions for ParallelNode.
type ParallelPolicy int

const (
	ParallelAllSuccess ParallelPolicy = iota
	ParallelOneSuccess
)

// ParallelNode executes children according to a policy.
type ParallelNode struct {
	children []Node
	policy   ParallelPolicy
}

func (n *ParallelNode) AddChild(c Node) { n.children = append(n.children, c) }

func (n *ParallelNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	successes := 0
	running := false
	for _, child := range n.children {
		st := child.Tick(ctx, bb)
		switch st {
		case core.StatusSuccess:
			successes++
		case core.StatusRunning:
			running = true
		case core.StatusFailure:
			if n.policy == ParallelAllSuccess {
				return core.StatusFailure
			}
		}
	}
	switch n.policy {
	case ParallelAllSuccess:
		if successes == len(n.children) {
			return core.StatusSuccess
		}
	case ParallelOneSuccess:
		if successes > 0 {
			return core.StatusSuccess
		}
	}
	if running {
		return core.StatusRunning
	}
	return core.StatusFailure
}

func (n *ParallelNode) Reset() {
	for _, c := range n.children {
		c.Reset()
	}
}

// RepeatNode repeats its child a fixed number of times.
type RepeatNode struct {
	child Node
	times int
	count int
}

func NewRepeatNode(child Node, times int) *RepeatNode {
	return &RepeatNode{child: child, times: times}
}

func (n *RepeatNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	if n.times > 0 && n.count >= n.times {
		return core.StatusSuccess
	}
	st := n.child.Tick(ctx, bb)
	if st == core.StatusSuccess || st == core.StatusFailure {
		n.child.Reset()
		n.count++
	}
	if n.times > 0 && n.count >= n.times {
		return core.StatusSuccess
	}
	return core.StatusRunning
}

func (n *RepeatNode) Reset() {
	n.child.Reset()
	n.count = 0
}

// InverterNode inverts the result of its child.
type InverterNode struct {
	child Node
}

func NewInverterNode(child Node) *InverterNode { return &InverterNode{child: child} }

func (n *InverterNode) Tick(ctx context.Context, bb blackboard.Store) Status {
	st := n.child.Tick(ctx, bb)
	switch st {
	case core.StatusSuccess:
		return core.StatusFailure
	case core.StatusFailure:
		return core.StatusSuccess
	default:
		return st
	}
}

func (n *InverterNode) Reset() { n.child.Reset() }

// BehaviorTreeAgent periodically ticks the root node.
type BehaviorTreeAgent struct {
	id         string
	root       Node
	bus        eventbus.Bus
	blackboard blackboard.Store
	ticker     *time.Ticker
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *log.Logger
}

// NewBehaviorTreeAgent returns a new agent instance.
func NewBehaviorTreeAgent(id string, root Node, bus eventbus.Bus, bb blackboard.Store, logger *log.Logger) *BehaviorTreeAgent {
	if logger == nil {
		logger = log.Default()
	}
	return &BehaviorTreeAgent{id: id, root: root, bus: bus, blackboard: bb, logger: logger}
}

func (a *BehaviorTreeAgent) ID() string { return a.id }

func (a *BehaviorTreeAgent) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.ticker = time.NewTicker(time.Second)
	go a.run()
	return nil
}

func (a *BehaviorTreeAgent) run() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.ticker.C:
			_ = a.root.Tick(a.ctx, a.blackboard)
		}
	}
}

func (a *BehaviorTreeAgent) Stop(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.ticker != nil {
		a.ticker.Stop()
	}
	return nil
}

func (a *BehaviorTreeAgent) HandleEvent(ev core.Event) error {
	_ = ev
	return nil
}
