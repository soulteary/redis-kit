package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides a Redis-based cache implementation
type RedisCache struct {
	client    *redis.Client
	keyPrefix string
}

// NewCache creates a new Redis cache with the given client and key prefix
func NewCache(client *redis.Client, keyPrefix string) *RedisCache {
	return &RedisCache{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// buildKey constructs the full key with prefix
func (c *RedisCache) buildKey(key string) string {
	if c.keyPrefix == "" {
		return key
	}
	return c.keyPrefix + key
}

// Set stores a value in Redis with the given TTL
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)

	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Store in Redis with TTL
	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// Get retrieves a value from Redis
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)

	// Get from Redis
	data, err := c.client.Get(ctx, fullKey).Bytes()
	if err == redis.Nil {
		return fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		return fmt.Errorf("failed to get cache: %w", err)
	}

	// Deserialize from JSON
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Del deletes a key from Redis
func (c *RedisCache) Del(ctx context.Context, key string) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)
	return c.client.Del(ctx, fullKey).Err()
}

// Exists checks if a key exists in Redis
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)
	count, err := c.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return count > 0, nil
}

// TTL returns the remaining time-to-live of a key
func (c *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if c.client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)
	ttl, err := c.client.TTL(ctx, fullKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get TTL: %w", err)
	}

	return ttl, nil
}

// Expire sets the expiration time for a key
func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	fullKey := c.buildKey(key)
	return c.client.Expire(ctx, fullKey, ttl).Err()
}
