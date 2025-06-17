# Hybrid Agent Architecture in Go

This repository provides a template for building a hybrid multi-agent system in Go. Redis is used both as a message bus and as shared storage. The design integrates several well known patterns: BDI (Belief-Desire-Intention), finite-state machines, behavior trees, the Subsumption architecture, the Contract Net protocol, a blackboard, an event bus, and a meta-agent controller.

The goal is to offer a ready-to-use skeleton that can be extended for research or production purposes.

## Architecture

```text
         +---------------------------+
         |    Meta-Agent Controller  |
         +---------------------------+
                   |
+----------------------------------------------+
|    Go Agent System (BDI, FSM, BT, ...)       |
|----------------------------------------------|
|  Event Bus (Redis Pub/Sub)   Blackboard     |
+----------------------------------------------+
   |                     |
+---------+       +--------------+
|  Redis  |       | RedisInsight |
+---------+       +--------------+
```

## Implemented Patterns

* **BDI (Belief-Desire-Intention)** – models agent intelligence via beliefs, desires and intentions, separating plan selection from execution.
* **FSM (Finite-State Machine)** – provides state and transition management for agent behavior.
* **Behavior Tree** – composes complex behavior from simple tasks using a tree structure.
* **Subsumption** – layered reactive architecture where higher levels may suppress lower level actions.
* **Contract Net Protocol** – task distribution via a manager broadcasting calls for proposals, collecting bids and selecting a winner.
* **Blackboard** – shared storage where agents publish and read world information and intermediate results.
* **Event Bus** – asynchronous message routing via publish/subscribe.
* **Meta-Agent** – supervises other agents, spawns new ones and ensures global goals are met.

## Running the System

### Docker Compose

```bash
docker-compose up -d
```

### Makefile

* `make run-redis` – start Redis in the background.
* `make run` – build and run the application with the default configuration.
* `make stop` – stop all Docker Compose services.

### CLI

When running locally you can specify a configuration file:

```bash
./bin/agentctl --config configs/default.yaml
```

## Demo Scenario

A Contract Net demo can be executed via:

```bash
make demo
```

1. A manager agent publishes a `CallForProposal` on the event bus.
2. Worker agents send their bids.
3. The manager selects the best bid and sends `AcceptProposal`; others receive `Reject`.
4. The chosen worker performs the task and reports the result on the blackboard or through the event bus.

This provides a quick example of task distribution and coordination.

## Additional Documentation

Refer to [docs/specification.md](docs/specification.md) for a full overview of the
project layout, module interfaces and development guidelines.

