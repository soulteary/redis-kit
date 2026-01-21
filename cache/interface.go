package cache

import (
	"context"
	"time"
)

// Cache provides a generic caching interface
type Cache interface {
	// Set stores a value in the cache with the given TTL
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Get retrieves a value from the cache
	// The dest parameter should be a pointer to the type you want to unmarshal into
	Get(ctx context.Context, key string, dest interface{}) error

	// Del deletes a key from the cache
	Del(ctx context.Context, key string) error

	// Exists checks if a key exists in the cache
	Exists(ctx context.Context, key string) (bool, error)

	// TTL returns the remaining time-to-live of a key
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Expire sets the expiration time for a key
	Expire(ctx context.Context, key string, ttl time.Duration) error
}
