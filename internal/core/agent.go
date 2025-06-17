package core

import "context"

// Agent defines the minimal behaviour expected from any agent.
type Agent interface {
	ID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HandleEvent(event Event) error
}
