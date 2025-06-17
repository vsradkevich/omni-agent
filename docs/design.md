# System Architecture and Design

## Architectural Rationale

This project follows a **hybrid approach** combining reactive and deliberative components. Reactive mechanisms such as Subsumption provide immediate responses to environmental changes, while planning patterns like BDI and behavior trees manage strategic goals and long term plans. Redis is used as the underlying backend for both the message bus and the shared blackboard. Redis Pub/Sub acts as the event bus, and key-value storage is used for the blackboard. This simplifies agent communication and knowledge storage in a scalable manner.

## Communication: Event Bus and Blackboard

The **Event Bus** is implemented via Redis Pub/Sub. Agents publish events and subscribe to channels of interest. This decouples senders from receivers: publishers are unaware of which agents will handle messages, and subscribers only receive relevant events. It makes the system extensible and easy to add new message types.

The **Blackboard** is a shared knowledge base using Redis. Agents store and read environmental state and intermediate results, similar to a shared bulletin board. This is useful for dividing work among subsystems: for instance, one agent writes sensor output while others read it for planning.

The combination of the event bus and blackboard allows agents to exchange information efficiently: signaling messages are propagated on the bus, while persistent context is stored on the blackboard.

## Subsystems and Interactions

- **Agents** are the primary unit of the system. Each agent includes several components:
  - A **BDI model** to hold beliefs and formulate goals.
  - An **FSM** to manage current agent state.
  - A **Behavior Tree** for composing sequences of actions.
  - A **Subsumption layer** for low level reactions, such as obstacle avoidance.
  - Communication modules for the event bus and access to the blackboard.

  Agents coordinate with each other. For instance, a manager agent may distribute tasks and workers execute them. All agents publish events and interact with the blackboard as needed.

- **Event Bus** uses Redis Pub/Sub for message exchange without tight coupling. Contract Net messages (CallForProposal, Bid, Accept/Reject) are sent over the bus.

- **Blackboard** is the shared database where agents store environment state and results. Subsystems write information such as completed tasks so others can consume it during decision making.

- **Contract Net** distributes tasks dynamically. A manager broadcasts a new job on the bus, workers respond with bids, the manager accepts the best bid and notifies the chosen worker. The worker executes the job and writes results to the blackboard.

- **Meta-Agent** monitors the blackboard and global objectives. It can spawn new agents or launch additional Contract Net rounds based on the overall state. This provides high-level coordination and adaptation.

## Pattern Implementation

- **BDI**: Agents store beliefs updated from events or sensors. Desires arise from new events, and the planner chooses actionable intentions.

- **FSM**: Each agent behavior is implemented as a finite-state machine with states like `Idle`, `Seeking`, `Executing`. Transitions occur based on conditions from the event bus or blackboard.

- **Behavior Tree**: Complex tasks are decomposed into sequences defined in a behavior tree, making it easy to compose advanced behaviors from simple actions.

- **Subsumption**: The lowest reactive layer can suppress higher level plans in critical situations, ensuring quick responses regardless of the current intention.

- **Contract Net Protocol**: Implemented via event bus messages. A manager sends `CallForProposals`, workers respond with `Propose` or `Refuse`, and the manager sends `AcceptProposal` or `RejectProposal` accordingly. Message formats are based on the FIPA standard but implemented using our bus.

- **Blackboard**: The central repository for maps, plans and results. Agents record information such as `planResult: success` and read it to synchronize their work.

- **Event Bus**: All communication goes through publish/subscribe channels, simplifying integration of new agents and services.

- **Meta-Agent**: Periodically checks the blackboard and may initiate new planning or Contract Net rounds when it detects new requirements.

