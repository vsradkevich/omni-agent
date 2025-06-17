package contractnet

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/core"
	"go-agent-framework/internal/eventbus"
)

// MessageType defines the kind of contract net message.
type MessageType string

const (
	MsgCallForProposal MessageType = "cfp"
	MsgPropose         MessageType = "propose"
	MsgReject          MessageType = "reject"
	MsgAccept          MessageType = "accept"
	MsgInform          MessageType = "inform"
)

// TaskSpec describes a task offered by the manager.
type TaskSpec struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description"`
	Deadline     time.Time              `json:"deadline"`
	Requirements map[string]interface{} `json:"requirements"`
}

// Proposal submitted by a contractor.
type Proposal struct {
	ID           string        `json:"id"`
	TaskID       string        `json:"task_id"`
	ContractorID string        `json:"contractor_id"`
	Cost         float64       `json:"cost"`
	Duration     time.Duration `json:"duration"`
	Confidence   float64       `json:"confidence"`
}

// ContractMessage is exchanged via the event bus.
type ContractMessage struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	TaskID    string      `json:"task_id"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	TaskSpec  *TaskSpec   `json:"task_spec,omitempty"`
	Proposal  *Proposal   `json:"proposal,omitempty"`
	Result    interface{} `json:"result,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

func encodeMessage(msg ContractMessage) core.Event {
	data, _ := json.Marshal(msg)
	return core.Event{
		ID:        msg.ID,
		Type:      string(msg.Type),
		Source:    msg.From,
		Timestamp: msg.Timestamp,
		Payload:   map[string]interface{}{"msg": string(data)},
	}
}

func decodeMessage(ev core.Event) (ContractMessage, error) {
	var msg ContractMessage
	if s, ok := ev.Payload["msg"].(string); ok {
		err := json.Unmarshal([]byte(s), &msg)
		return msg, err
	}
	return msg, nil
}

// ----------------------------------------------------------------------------
// Manager
// ----------------------------------------------------------------------------

type ProposalEvaluator interface {
	Evaluate(proposals []*Proposal) *Proposal
}

type Manager struct {
	id              string
	eventBus        eventbus.Bus
	blackboard      blackboard.Store
	proposalTimeout time.Duration
	evaluator       ProposalEvaluator

	mu        sync.Mutex
	tasks     map[string]*TaskSpec
	proposals map[string][]*Proposal
	ctx       context.Context
	cancel    context.CancelFunc
	logger    *log.Logger
}

func NewManager(id string, bus eventbus.Bus, bb blackboard.Store, evaluator ProposalEvaluator, logger *log.Logger) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{
		id:              id,
		eventBus:        bus,
		blackboard:      bb,
		evaluator:       evaluator,
		proposalTimeout: time.Second,
		tasks:           make(map[string]*TaskSpec),
		proposals:       make(map[string][]*Proposal),
		logger:          logger,
	}
}

func (m *Manager) ID() string { return m.id }

func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	ch, err := m.eventBus.Subscribe(m.ctx, "contractnet.propose")
	if err != nil {
		return err
	}
	go m.run(ch)
	return nil
}

func (m *Manager) run(ch <-chan core.Event) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			msg, err := decodeMessage(ev)
			if err != nil {
				m.logger.Println("decode", err)
				continue
			}
			if msg.Type == MsgPropose {
				m.mu.Lock()
				m.proposals[msg.TaskID] = append(m.proposals[msg.TaskID], msg.Proposal)
				m.mu.Unlock()
			}
		}
	}
}

func (m *Manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	return m.eventBus.Unsubscribe(context.Background(), "contractnet.propose")
}

func (m *Manager) AnnounceTask(spec TaskSpec) error {
	if time.Now().After(spec.Deadline) {
		return fmt.Errorf("task %s deadline exceeded", spec.ID)
	}
	m.mu.Lock()
	m.tasks[spec.ID] = &spec
	m.mu.Unlock()
	msg := ContractMessage{
		ID:        uuid.NewString(),
		Type:      MsgCallForProposal,
		TaskID:    spec.ID,
		From:      m.id,
		TaskSpec:  &spec,
		Timestamp: time.Now(),
	}
	err := m.eventBus.Publish(m.ctx, "contractnet.cfp", encodeMessage(msg))
	if err == nil {
		go func(id string) {
			time.Sleep(m.proposalTimeout)
			m.evaluate(id)
		}(spec.ID)
	}
	return err
}

func (m *Manager) evaluate(taskID string) {
	m.mu.Lock()
	proposals := m.proposals[taskID]
	spec := m.tasks[taskID]
	m.mu.Unlock()
	if spec != nil && time.Now().After(spec.Deadline) {
		m.logger.Println("task deadline passed", taskID)
		return
	}
	if len(proposals) == 0 {
		m.logger.Println("no proposals for", taskID)
		return
	}
	win := m.evaluator.Evaluate(proposals)
	if win == nil {
		return
	}
	accept := ContractMessage{
		ID:        uuid.NewString(),
		Type:      MsgAccept,
		TaskID:    taskID,
		From:      m.id,
		To:        win.ContractorID,
		TaskSpec:  spec,
		Timestamp: time.Now(),
	}
	_ = m.eventBus.Publish(m.ctx, "contractnet.contractor."+win.ContractorID, encodeMessage(accept))
	for _, p := range proposals {
		if p.ContractorID == win.ContractorID {
			continue
		}
		rej := ContractMessage{
			ID:        uuid.NewString(),
			Type:      MsgReject,
			TaskID:    taskID,
			From:      m.id,
			To:        p.ContractorID,
			Timestamp: time.Now(),
		}
		_ = m.eventBus.Publish(m.ctx, "contractnet.contractor."+p.ContractorID, encodeMessage(rej))
	}
}

func (m *Manager) HandleEvent(ev core.Event) error {
	msg, err := decodeMessage(ev)
	if err != nil {
		return err
	}
	if msg.Type == MsgInform {
		key := "contract:" + msg.TaskID
		_, err := m.blackboard.Put(m.ctx, key, msg.Result, 0)
		return err
	}
	return nil
}

// WaitAndEvaluate waits for proposals then chooses a contractor.
func (m *Manager) WaitAndEvaluate(taskID string) {
	time.Sleep(m.proposalTimeout)
	m.evaluate(taskID)
}

// ----------------------------------------------------------------------------
// Contractor
// ----------------------------------------------------------------------------

type BidStrategy interface {
	ShouldBid(task *TaskSpec, cap map[string]float64) bool
	GenerateProposal(task *TaskSpec, contractorID string) *Proposal
}

type TaskExecutor interface {
	Execute(ctx context.Context, task *TaskSpec) (interface{}, error)
}

type Contractor struct {
	id           string
	capabilities map[string]float64
	currentTasks map[string]*TaskSpec
	maxTasks     int
	bidStrategy  BidStrategy
	executor     TaskExecutor

	eventBus   eventbus.Bus
	blackboard blackboard.Store

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	logger *log.Logger
}

func NewContractor(id string, caps map[string]float64, bus eventbus.Bus, bb blackboard.Store, strat BidStrategy, exec TaskExecutor, logger *log.Logger) *Contractor {
	if logger == nil {
		logger = log.Default()
	}
	return &Contractor{
		id:           id,
		capabilities: caps,
		maxTasks:     1,
		bidStrategy:  strat,
		executor:     exec,
		eventBus:     bus,
		blackboard:   bb,
		currentTasks: make(map[string]*TaskSpec),
		logger:       logger,
	}
}

func (c *Contractor) ID() string { return c.id }

func (c *Contractor) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	cfpCh, err := c.eventBus.Subscribe(c.ctx, "contractnet.cfp")
	if err != nil {
		return err
	}
	respCh, err := c.eventBus.Subscribe(c.ctx, "contractnet.contractor."+c.id)
	if err != nil {
		return err
	}
	go c.runCFP(cfpCh)
	go c.runResp(respCh)
	return nil
}

func (c *Contractor) runCFP(ch <-chan core.Event) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			msg, err := decodeMessage(ev)
			if err != nil || msg.Type != MsgCallForProposal {
				continue
			}
			if c.bidStrategy != nil && !c.bidStrategy.ShouldBid(msg.TaskSpec, c.capabilities) {
				continue
			}
			prop := c.bidStrategy.GenerateProposal(msg.TaskSpec, c.id)
			prop.ID = uuid.NewString()
			out := ContractMessage{
				ID:        uuid.NewString(),
				Type:      MsgPropose,
				TaskID:    msg.TaskID,
				From:      c.id,
				Proposal:  prop,
				Timestamp: time.Now(),
			}
			_ = c.eventBus.Publish(c.ctx, "contractnet.propose", encodeMessage(out))
		}
	}
}

func (c *Contractor) runResp(ch <-chan core.Event) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			msg, err := decodeMessage(ev)
			if err != nil {
				continue
			}
			switch msg.Type {
			case MsgAccept:
				c.handleAccept(msg)
			case MsgReject:
				// ignore for now
			}
		}
	}
}

func (c *Contractor) handleAccept(msg ContractMessage) {
	c.mu.Lock()
	if len(c.currentTasks) >= c.maxTasks {
		c.mu.Unlock()
		return
	}
	task := msg.TaskSpec
	c.currentTasks[task.ID] = task
	c.mu.Unlock()
	go func() {
		if time.Now().After(task.Deadline) {
			inf := ContractMessage{
				ID:        uuid.NewString(),
				Type:      MsgInform,
				TaskID:    task.ID,
				From:      c.id,
				To:        msg.From,
				Result:    "deadline exceeded",
				Timestamp: time.Now(),
			}
			_ = c.eventBus.Publish(c.ctx, "contractnet.inform", encodeMessage(inf))
			return
		}
		result, err := c.executor.Execute(c.ctx, task)
		if err != nil {
			result = err.Error()
		}
		inf := ContractMessage{
			ID:        uuid.NewString(),
			Type:      MsgInform,
			TaskID:    task.ID,
			From:      c.id,
			To:        msg.From,
			Result:    result,
			Timestamp: time.Now(),
		}
		_ = c.eventBus.Publish(c.ctx, "contractnet.inform", encodeMessage(inf))
	}()
}

func (c *Contractor) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.eventBus.Unsubscribe(context.Background(), "contractnet.cfp")
	_ = c.eventBus.Unsubscribe(context.Background(), "contractnet.contractor."+c.id)
	return nil
}

func (c *Contractor) HandleEvent(ev core.Event) error {
	_ = ev
	return nil
}
