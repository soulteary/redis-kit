package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultLockTime is the default lock expiration time (15 seconds)
	DefaultLockTime = 15 * time.Second

	// DefaultOperationTimeout is the default timeout for lock operations (5 seconds)
	DefaultOperationTimeout = 5 * time.Second
)

// RedisLocker provides Redis-based distributed lock functionality
type RedisLocker struct {
	client    *redis.Client
	lockTime  time.Duration
	lockStore sync.Map // Stores key -> lockValue mapping
}

// NewRedisLocker creates a new Redis-based distributed locker
func NewRedisLocker(client *redis.Client) *RedisLocker {
	return NewRedisLockerWithLockTime(client, DefaultLockTime)
}

// NewRedisLockerWithLockTime creates a new Redis-based distributed locker with custom lock time
func NewRedisLockerWithLockTime(client *redis.Client, lockTime time.Duration) *RedisLocker {
	return &RedisLocker{
		client:   client,
		lockTime: lockTime,
	}
}

// generateLockValue generates a unique lock value
func generateLockValue() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Lock acquires a distributed lock using Redis SETNX
// Returns true if the lock was successfully acquired, false if the lock is already held
func (r *RedisLocker) Lock(key string) (bool, error) {
	if r.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}

	lockValue, err := generateLockValue()
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()

	res, err := r.client.SetNX(ctx, key, lockValue, r.lockTime).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if res {
		// Store lockValue for subsequent unlock verification
		r.lockStore.Store(key, lockValue)
	}

	return res, nil
}

// Unlock releases a distributed lock using a Lua script to ensure atomicity
// Only releases the lock if the lock value matches, preventing accidental release of another process's lock
func (r *RedisLocker) Unlock(key string) error {
	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	// Get stored lockValue
	value, ok := r.lockStore.LoadAndDelete(key)
	if !ok {
		return ErrLockNotHeld
	}

	lockValue, ok := value.(string)
	if !ok {
		return ErrLockValueType
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()

	// Use Lua script to ensure atomicity: only delete when lock value matches
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	result, err := r.client.Eval(ctx, script, []string{key}, lockValue).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	// Check if lock was actually released
	if val, ok := result.(int64); !ok || val == 0 {
		return ErrLockValueMismatch
	}

	return nil
}

// HybridLocker provides distributed lock functionality with automatic fallback to local lock
// If Redis is unavailable or operations fail, it automatically falls back to local lock
type HybridLocker struct {
	redisLocker *RedisLocker
	localLocker *LocalLocker
}

// NewHybridLocker creates a new hybrid locker that supports both Redis and local locking
// If client is nil, it will only use local locking
func NewHybridLocker(client *redis.Client) *HybridLocker {
	hl := &HybridLocker{
		localLocker: NewLocalLocker(),
	}

	if client != nil {
		hl.redisLocker = NewRedisLocker(client)
	}

	return hl
}

// Lock acquires a lock, trying Redis first and falling back to local lock if Redis fails
func (h *HybridLocker) Lock(key string) (bool, error) {
	// Try Redis first if available
	if h.redisLocker != nil {
		success, err := h.redisLocker.Lock(key)
		if err == nil {
			return success, nil
		}
		// If Redis fails, fall back to local lock
	}

	// Fall back to local lock
	return h.localLocker.Lock(key)
}

// Unlock releases a lock, trying Redis first and falling back to local lock if Redis fails
func (h *HybridLocker) Unlock(key string) error {
	// Try Redis first if available
	if h.redisLocker != nil {
		// Check if this key was locked via Redis by checking if it exists in lockStore
		// We can't directly check, so we try Redis unlock first
		err := h.redisLocker.Unlock(key)
		if err == nil {
			return nil
		}
		// If Redis unlock fails due to lock value mismatch or lock expired,
		// we should return the error instead of falling back to local lock
		// Only fall back to local lock for connection/network errors
		if errors.Is(err, ErrLockValueMismatch) || errors.Is(err, ErrLockValueType) {
			return err
		}
		// For other errors (e.g., connection failures), try local unlock
		if localErr := h.localLocker.Unlock(key); localErr == nil {
			return nil
		}
		return err
	}

	// Fall back to local lock
	return h.localLocker.Unlock(key)
}
