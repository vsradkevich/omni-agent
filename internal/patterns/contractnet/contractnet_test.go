package contractnet

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"go-agent-framework/internal/blackboard"
	"go-agent-framework/internal/eventbus"
)

type simpleEvaluator struct{}

func (simpleEvaluator) Evaluate(props []*Proposal) *Proposal {
	if len(props) == 0 {
		return nil
	}
	return props[0]
}

type alwaysBid struct{}

func (alwaysBid) ShouldBid(task *TaskSpec, caps map[string]float64) bool { return true }
func (alwaysBid) GenerateProposal(task *TaskSpec, contractorID string) *Proposal {
	return &Proposal{TaskID: task.ID, ContractorID: contractorID, Cost: 1}
}

type dummyExec struct{}

func (dummyExec) Execute(ctx context.Context, t *TaskSpec) (interface{}, error) {
	return "done", nil
}

func TestContractNetCycle(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	bus := eventbus.NewRedisBus(&redis.Options{Addr: mr.Addr()}, nil)
	bb := blackboard.NewRedisStore(&redis.Options{Addr: mr.Addr()}, nil)

	mgr := NewManager("mgr", bus, bb, simpleEvaluator{}, nil)
	mgr.proposalTimeout = 50 * time.Millisecond
	cont := NewContractor("c1", nil, bus, bb, alwaysBid{}, dummyExec{}, nil)
	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	if err := cont.Start(ctx); err != nil {
		t.Fatalf("contractor start: %v", err)
	}
	evCh, err := bus.Subscribe(ctx, "contractnet.inform")
	if err != nil {
		t.Fatalf("subscribe inform: %v", err)
	}
	task := TaskSpec{ID: "t1", Description: "demo", Deadline: time.Now().Add(time.Second)}
	if err := mgr.AnnounceTask(task); err != nil {
		t.Fatalf("announce: %v", err)
	}
	select {
	case ev := <-evCh:
		msg, _ := decodeMessage(ev)
		if msg.Type != MsgInform || msg.TaskID != task.ID {
			t.Fatalf("unexpected inform %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inform")
	}
	mgr.Stop(ctx)
	cont.Stop(ctx)
}

func TestAnnounceTaskDeadline(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	bus := eventbus.NewRedisBus(&redis.Options{Addr: mr.Addr()}, nil)
	bb := blackboard.NewRedisStore(&redis.Options{Addr: mr.Addr()}, nil)

	mgr := NewManager("mgr", bus, bb, simpleEvaluator{}, nil)
	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	defer mgr.Stop(ctx)

	task := TaskSpec{ID: "t2", Description: "late", Deadline: time.Now().Add(-time.Second)}
	if err := mgr.AnnounceTask(task); err == nil {
		t.Fatal("expected error for expired deadline")
	}
}
