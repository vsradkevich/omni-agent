package eventbus

import "context"
import "go-agent-framework/internal/core"

// Bus defines publish/subscribe semantics for events.
type Bus interface {
	Publish(ctx context.Context, topic string, event core.Event) error
	Subscribe(ctx context.Context, topic string) (<-chan core.Event, error)
	SubscribePattern(ctx context.Context, pattern string) (<-chan core.Event, error)
	Unsubscribe(ctx context.Context, topic string) error
	Close() error
}
