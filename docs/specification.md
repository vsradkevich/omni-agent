# Project Specification

This document describes the goals, directory layout and module design for the Go + Redis multi-agent system.

## Goals

- Provide a minimal yet functional framework implementing the following patterns:
  - Belief--Desire--Intention (BDI)
  - Finite-State Machine (FSM)
  - Behavior Trees
  - Subsumption layers
  - Contract Net Protocol (CNP)
  - Blackboard for shared data
  - Event Bus via Redis Pub/Sub
  - Meta-Agent for high level orchestration
- Offer clear extension points so developers can add custom agents or behaviors.
- Supply build and run scripts that work out of the box using Docker Compose.

## Repository Layout

```
cmd/agentctl/             CLI entrypoint and command definitions
internal/core/            Base agent interfaces and common types
internal/eventbus/        Redis-based event bus
internal/blackboard/      Shared knowledge store
internal/patterns/bdi/    BDI implementation
internal/patterns/fsm/    Finite state machine logic
internal/patterns/bt/     Behavior tree nodes and runner
internal/patterns/subsumption/  Subsumption behaviors
internal/patterns/contractnet/  Contract Net Protocol actors
internal/meta/            Meta-agent orchestrator
configs/               YAML configuration files
Dockerfile             Build instructions for the application image
docker-compose.yml     Services for Redis and the application
Makefile               Convenience targets for build and test
docs/                  Project documentation
```

## Module Outline

Each directory under `internal/` represents a standalone module. Key interfaces are summarised below.

### internal/core
```go
// Agent defines the minimal behaviour expected from any agent.
type Agent interface {
    ID() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HandleEvent(event Event) error
}
```
The core package also defines basic event structures and status types used by other modules.

### internal/bdi
Implements a BDI agent.
```go
type BDIAgent struct {
    Beliefs map[string]interface{}
    Desires []Goal
    Intentions []Plan
}
```
The planner selects a `Plan` based on current beliefs and desires. Plans consist of steps executed in order.

### internal/fsm
Provides a generic finite state machine.
```go
type State string

type Transition struct {
    Event string
    Next State
}
```
`StateMachine` maps states and events to transitions and executes optional actions during transitions.

### internal/bt
Defines behaviour tree nodes.
```go
type Node interface {
    Tick(ctx context.Context) Status
}
```
Composite nodes (sequence, selector, parallel) manage child nodes; leaf nodes implement concrete actions or conditions.

### internal/subsumption
Implements layered behaviour control where higher priority layers can override lower ones.
```go
type Layer interface {
    Priority() int
    Execute(ctx context.Context) bool
}
```
Layers are checked in order of priority each cycle.

### internal/contractnet
Actors participating in the Contract Net protocol exchange typed messages over the event bus.
```go
type CallForProposal struct{ ... }
```
Manager agents gather proposals and award contracts; worker agents respond to calls and execute tasks.

### internal/blackboard
Shared data store with a simple API.
```go
type Blackboard interface {
    Write(ctx context.Context, key string, value any) error
    Read(ctx context.Context, key string) (any, error)
}
```
A Redis-backed implementation will be provided.

### internal/eventbus
Wrapper around Redis Pub/Sub.
```go
func Publish(ctx context.Context, channel string, msg any) error
func Subscribe(ctx context.Context, channel string) (<-chan Event, error)
```
Events may be encoded as JSON.

### internal/meta
The meta-agent monitors running agents and can spawn or terminate them.
```go
type MetaAgent struct {
    registry map[string]agent.Agent
}
```

## Testing Strategy

- Each module includes unit tests exercising its core logic.
- Integration tests demonstrate cooperation between modules. Example: a Contract Net round using the event bus and blackboard.
- `go test ./...` runs all tests. The repository includes a `go.mod` file so tooling works correctly.

## CLI

The `agentctl` binary exposes several commands.

- `agentctl start` – launch Redis and the agent application via Docker Compose.
- `agentctl stop` – stop the services.
- `agentctl status` – display currently running agents.
- `agentctl spawn-agent <type>` – request creation of a new agent instance.
- `agentctl simulate <scenario>` – run a predefined demonstration scenario.

## Running the System

1. Ensure Docker and Docker Compose are installed.
2. Build the application container:
   ```bash
   make build
   ```
3. Start Redis and the application:
   ```bash
   docker-compose up -d
   ```
4. Use `agentctl` to interact with the system.

## Future Work

The current repository provides only scaffolding. Implementations of the modules above are left to contributors. The design aims to keep each module independent and easy to extend.

