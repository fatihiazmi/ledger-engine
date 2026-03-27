package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRequestInFlight = errors.New("request is already being processed")

const DefaultTTL = 24 * time.Hour

// IdempotencyStore tracks request keys in Redis to prevent duplicate processing.
// If a key exists, the cached response is returned instead of re-executing.
type IdempotencyStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewIdempotencyStore(client *redis.Client) *IdempotencyStore {
	return &IdempotencyStore{client: client, ttl: DefaultTTL}
}

// Check returns the cached response if the key was already processed.
// Returns nil if the key is new (caller should proceed with the request).
func (s *IdempotencyStore) Check(ctx context.Context, key string) ([]byte, error) {
	val, err := s.client.Get(ctx, idempotencyKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil // key doesn't exist — new request
	}
	if err != nil {
		return nil, err
	}
	return val, nil // cached response found
}

// Lock marks a key as in-flight using SET NX (only succeeds if key doesn't exist).
// Returns true if lock acquired, false if key already exists.
func (s *IdempotencyStore) Lock(ctx context.Context, key string) (bool, error) {
	ok, err := s.client.SetNX(ctx, idempotencyLock(key), "processing", 30*time.Second).Result()
	return ok, err
}

// Store saves the response for a completed request and removes the lock.
func (s *IdempotencyStore) Store(ctx context.Context, key string, response []byte, statusCode int) error {
	pipe := s.client.Pipeline()

	// Store response with TTL
	pipe.Set(ctx, idempotencyKey(key), response, s.ttl)
	pipe.Set(ctx, idempotencyStatus(key), statusCode, s.ttl)

	// Remove lock
	pipe.Del(ctx, idempotencyLock(key))

	_, err := pipe.Exec(ctx)
	return err
}

// GetStatus returns the cached HTTP status code for a key.
func (s *IdempotencyStore) GetStatus(ctx context.Context, key string) (int, error) {
	val, err := s.client.Get(ctx, idempotencyStatus(key)).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return val, err
}

// Unlock removes the lock if processing fails (so retries can proceed).
func (s *IdempotencyStore) Unlock(ctx context.Context, key string) error {
	return s.client.Del(ctx, idempotencyLock(key)).Err()
}

func idempotencyKey(key string) string    { return "idem:" + key }
func idempotencyLock(key string) string   { return "idem:lock:" + key }
func idempotencyStatus(key string) string { return "idem:status:" + key }
