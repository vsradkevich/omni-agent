package blackboard

import (
	"context"
	"time"

	"go-agent-framework/internal/core"
)

// Store defines operations for a shared knowledge base.
type Store interface {
	Put(ctx context.Context, key string, value interface{}, ttl time.Duration) (int64, error)
	Get(ctx context.Context, key string) (interface{}, int64, error)
	Txn(ctx context.Context, values map[string]interface{}, ttl time.Duration) error
	Watch(ctx context.Context, pattern string) (<-chan core.BlackboardUpdate, error)
	Delete(ctx context.Context, key string) error
	Close() error
}
