package blackboard

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go-agent-framework/internal/core"
)

// item represents stored data with version.
type item struct {
	Version int64       `json:"version"`
	Value   interface{} `json:"value"`
}

// RedisStore provides a Redis-backed implementation of Store.
type RedisStore struct {
	mu       sync.Mutex
	client   *redis.Client
	options  *redis.Options
	logger   *log.Logger
	notifKey string
}

// NewRedisStore returns a new RedisStore with given options.
func NewRedisStore(opts *redis.Options, logger *log.Logger) *RedisStore {
	if logger == nil {
		logger = log.Default()
	}
	return &RedisStore{
		client:   redis.NewClient(opts),
		options:  opts,
		logger:   logger,
		notifKey: "blackboard:update:",
	}
}

// ensureConnection pings Redis and reconnects if needed.
func (s *RedisStore) ensureConnection(ctx context.Context) {
	if err := s.client.Ping(ctx).Err(); err != nil {
		s.logger.Println("blackboard reconnecting to Redis", err)
		s.client = redis.NewClient(s.options)
	}
}

// Put stores a value with optional TTL and returns the new version.
func (s *RedisStore) Put(ctx context.Context, key string, value interface{}, ttl time.Duration) (int64, error) {
	s.ensureConnection(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	hkey := key
	var ver int64 = 1
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		res, _ := tx.HGet(ctx, hkey, "version").Int64()
		ver = res + 1
		data, _ := json.Marshal(value)
		pipe := tx.TxPipeline()
		pipe.HSet(ctx, hkey, "value", data, "version", ver)
		if ttl > 0 {
			pipe.Expire(ctx, hkey, ttl)
		}
		_, err := pipe.Exec(ctx)
		return err
	}, hkey)
	if err != nil {
		return 0, err
	}
	upd := core.BlackboardUpdate{Key: key, Value: value}
	payload, _ := json.Marshal(upd)
	s.client.Publish(ctx, s.notifKey+key, payload)
	return ver, nil
}

// Get retrieves a value and its version.
func (s *RedisStore) Get(ctx context.Context, key string) (interface{}, int64, error) {
	s.ensureConnection(ctx)
	res, err := s.client.HGetAll(ctx, key).Result()
	if err != nil || len(res) == 0 {
		return nil, 0, err
	}
	var v interface{}
	if err := json.Unmarshal([]byte(res["value"]), &v); err != nil {
		return nil, 0, err
	}
	ver, _ := parseInt(res["version"])
	return v, ver, nil
}

func parseInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// Txn performs multiple puts atomically.
func (s *RedisStore) Txn(ctx context.Context, values map[string]interface{}, ttl time.Duration) error {
	s.ensureConnection(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	pipe := s.client.TxPipeline()
	for k, v := range values {
		data, _ := json.Marshal(v)
		pipe.HIncrBy(ctx, k, "version", 1)
		pipe.HSet(ctx, k, "value", data)
		if ttl > 0 {
			pipe.Expire(ctx, k, ttl)
		}
		upd := core.BlackboardUpdate{Key: k, Value: v}
		payload, _ := json.Marshal(upd)
		pipe.Publish(ctx, s.notifKey+k, payload)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Watch subscribes to updates matching a pattern.
func (s *RedisStore) Watch(ctx context.Context, pattern string) (<-chan core.BlackboardUpdate, error) {
	s.ensureConnection(ctx)
	pubsub := s.client.PSubscribe(ctx, s.notifKey+pattern)
	ch := make(chan core.BlackboardUpdate)
	go func() {
		defer close(ch)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Println("blackboard watch error", err)
				time.Sleep(time.Second)
				continue
			}
			var upd core.BlackboardUpdate
			if err := json.Unmarshal([]byte(msg.Payload), &upd); err == nil {
				ch <- upd
			}
		}
	}()
	return ch, nil
}

// Delete removes a key from the store.
func (s *RedisStore) Delete(ctx context.Context, key string) error {
	s.ensureConnection(ctx)
	return s.client.Del(ctx, key).Err()
}

// Close closes the Redis connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}
