package eventbus

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go-agent-framework/internal/core"
)

// RedisBus implements Bus using Redis Pub/Sub with automatic reconnection.
type RedisBus struct {
	mu            sync.Mutex
	client        *redis.Client
	options       *redis.Options
	subscriptions map[string]*redis.PubSub
	logger        *log.Logger
}

// NewRedisBus creates a new Redis-backed event bus using the given options.
func NewRedisBus(opts *redis.Options, logger *log.Logger) *RedisBus {
	if logger == nil {
		logger = log.Default()
	}
	client := redis.NewClient(opts)
	return &RedisBus{
		client:        client,
		options:       opts,
		subscriptions: make(map[string]*redis.PubSub),
		logger:        logger,
	}
}

// ensureConnection pings the server and reconnects if necessary.
func (b *RedisBus) ensureConnection(ctx context.Context) {
	if err := b.client.Ping(ctx).Err(); err != nil {
		b.logger.Println("eventbus reconnecting to Redis", err)
		b.client = redis.NewClient(b.options)
	}
}

// Publish sends an event to a topic.
func (b *RedisBus) Publish(ctx context.Context, topic string, event core.Event) error {
	b.ensureConnection(ctx)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, topic, data).Err()
}

// subscribeInternal handles subscription logic with given pubsub.
func (b *RedisBus) subscribeInternal(ctx context.Context, pubsub *redis.PubSub) (<-chan core.Event, error) {
	ch := make(chan core.Event)
	go func() {
		defer close(ch)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				b.logger.Println("eventbus receive error", err)
				time.Sleep(time.Second)
				continue
			}
			var ev core.Event
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
				ch <- ev
			}
		}
	}()
	return ch, nil
}

// Subscribe listens for events on a topic.
func (b *RedisBus) Subscribe(ctx context.Context, topic string) (<-chan core.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureConnection(ctx)
	ps := b.client.Subscribe(ctx, topic)
	b.subscriptions[topic] = ps
	return b.subscribeInternal(ctx, ps)
}

// SubscribePattern listens for events using a pattern.
func (b *RedisBus) SubscribePattern(ctx context.Context, pattern string) (<-chan core.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureConnection(ctx)
	ps := b.client.PSubscribe(ctx, pattern)
	b.subscriptions[pattern] = ps
	return b.subscribeInternal(ctx, ps)
}

// Unsubscribe stops listening on a topic.
func (b *RedisBus) Unsubscribe(ctx context.Context, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ps, ok := b.subscriptions[topic]
	if !ok {
		return nil
	}
	delete(b.subscriptions, topic)
	return ps.Close()
}

// Close terminates all subscriptions and closes the client.
func (b *RedisBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ps := range b.subscriptions {
		_ = ps.Close()
	}
	b.subscriptions = make(map[string]*redis.PubSub)
	return b.client.Close()
}
