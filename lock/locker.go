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
	client   *redis.Client
	lockTime time.Duration

	// mu guards lockStore.
	mu sync.Mutex

	// lockStore maps key -> the outstanding acquisitions for that key, OLDEST
	// FIRST, for the legacy Lock/Unlock pair -- which identifies a lock by key
	// alone and so has nowhere else to keep the token.
	//
	// A queue rather than a single entry, because the same locker can acquire
	// a key again after its lease elapses: storing only the latest let the
	// FIRST holder's Unlock consume the SECOND holder's token and
	// compare-and-delete a lock it never held. Unlock takes the oldest
	// outstanding entry, so each caller gets back its own acquisition and a
	// late Unlock finds its own expired one.
	//
	// Prefer Acquire, which hands the token to the caller in a Handle and does
	// not use this map at all.
	lockStore map[string][]lockEntry
}

// lockEntry is one legacy acquisition: its token and when its lease elapses.
//
// Recording the expiry is what makes the legacy pair safe. Previously only the
// token was stored, keyed by lock key, so a second acquisition of the same key
// overwrote the first one's token -- and the first holder's Unlock then
// compare-and-deleted using the *second* holder's token, releasing a lock it
// never held. Redis's compare-and-delete was correct; the process-local cache
// in front of it was not.
type lockEntry struct {
	token   string
	expires time.Time
}

// maxOutstandingPerKey bounds the queue for one key.
//
// A balanced Lock/Unlock pair leaves an empty queue and the key is dropped, so
// this only matters for a process that acquires and never releases (a panic,
// an early return). Past the cap the oldest entry is discarded to bound
// memory; a straggler Unlock for it then reports ErrLockNotHeld.
const maxOutstandingPerKey = 1024

// NewRedisLocker creates a new Redis-based distributed locker
func NewRedisLocker(client *redis.Client) *RedisLocker {
	return NewRedisLockerWithLockTime(client, DefaultLockTime)
}

// NewRedisLockerWithLockTime creates a new Redis-based distributed locker with custom lock time
func NewRedisLockerWithLockTime(client *redis.Client, lockTime time.Duration) *RedisLocker {
	return &RedisLocker{
		client:    client,
		lockTime:  lockTime,
		lockStore: make(map[string][]lockEntry),
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

// Lock acquires a distributed lock using Redis SET key value NX
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

	_, err = r.client.SetArgs(ctx, key, lockValue, redis.SetArgs{Mode: "NX", TTL: r.lockTime}).Result()
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	res := (err == nil)
	if res {
		// Append rather than replace: a previous holder of this key may not
		// have unlocked yet, and its Unlock must find ITS entry, not this one.
		r.storeEntry(key, lockEntry{token: lockValue, expires: time.Now().Add(r.lockTime)})
	}

	return res, nil
}

// takeOldest removes and returns the oldest outstanding acquisition for key.
func (r *RedisLocker) takeOldest(key string) (lockEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := r.lockStore[key]
	if len(queue) == 0 {
		return lockEntry{}, false
	}

	entry := queue[0]
	if len(queue) == 1 {
		// Balanced usage leaves nothing behind.
		delete(r.lockStore, key)
	} else {
		r.lockStore[key] = queue[1:]
	}
	return entry, true
}

// Unlock releases a distributed lock using a Lua script to ensure atomicity
// Only releases the lock if the lock value matches, preventing accidental release of another process's lock
func (r *RedisLocker) Unlock(key string) error {
	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	// Take the OLDEST outstanding acquisition for this key, which is this
	// caller's own in the paired usage the legacy API assumes.
	entry, ok := r.takeOldest(key)
	if !ok {
		return ErrLockNotHeld
	}

	// If our lease already elapsed, the key in Redis is either gone or held by
	// somebody who acquired it after us. Reporting that without touching Redis
	// is what stops a late Unlock from deleting another holder's lock.
	if time.Now().After(entry.expires) {
		return ErrLockExpired
	}

	lockValue := entry.token

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

// HybridLocker locks through Redis when a client is configured, and through a
// process-local lock when one is not.
//
// It does NOT fall back to the local lock when Redis is merely failing, unless
// allowLocalFallback is set. A process-local lock provides no mutual exclusion
// between instances, so degrading to it during a Redis outage silently drops
// the guarantee at exactly the moment it matters -- and the caller sees
// (true, nil), indistinguishable from a real distributed lock. Use
// NewHybridLockerWithLocalFallback to opt back in where losing exclusion is
// genuinely acceptable (a cache warmer, say, but never a payment or a coupon).
type HybridLocker struct {
	redisLocker        *RedisLocker
	localLocker        *LocalLocker
	allowLocalFallback bool
}

// NewHybridLocker creates a hybrid locker that uses Redis when client is
// non-nil and a process-local lock when it is nil.
//
// If a Redis operation fails, the error is returned rather than silently
// degrading to the local lock.
func NewHybridLocker(client *redis.Client) *HybridLocker {
	hl := &HybridLocker{
		localLocker: NewLocalLocker(),
	}

	if client != nil {
		hl.redisLocker = NewRedisLocker(client)
	}

	return hl
}

// NewHybridLockerWithLocalFallback is NewHybridLocker with the pre-existing
// behaviour of falling back to a process-local lock when Redis fails.
//
// Mutual exclusion across instances is lost while the fallback is in effect.
// Only use it where a concurrent second holder is acceptable.
func NewHybridLockerWithLocalFallback(client *redis.Client) *HybridLocker {
	hl := NewHybridLocker(client)
	hl.allowLocalFallback = true
	return hl
}

// Lock acquires a lock through Redis when configured.
//
// On a Redis failure it returns the error, so the caller can fail closed.
// Only a locker built with NewHybridLockerWithLocalFallback degrades to the
// process-local lock instead.
func (h *HybridLocker) Lock(key string) (bool, error) {
	if h.redisLocker != nil {
		success, err := h.redisLocker.Lock(key)
		if err == nil {
			return success, nil
		}
		if !h.allowLocalFallback {
			return false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
		}
		// Explicitly opted in: mutual exclusion across instances is now lost.
	}

	return h.localLocker.Lock(key)
}

// Unlock releases a lock, trying Redis first and falling back to local lock if Redis fails
func (h *HybridLocker) Unlock(key string) error {
	// Try Redis first if available
	if h.redisLocker != nil {
		if !h.allowLocalFallback {
			return h.redisLocker.Unlock(key)
		}
		// Check if this key was locked via Redis by checking if it exists in lockStore
		// We can't directly check, so we try Redis unlock first
		err := h.redisLocker.Unlock(key)
		if err == nil {
			return nil
		}
		// A verdict about a lock this locker DID hold is returned as-is;
		// only an unreachable Redis may fall through to the local lock.
		//
		// ErrLockExpired used to fall through, and that was a lock-stealing
		// path: a first holder whose Redis lease had elapsed, with Redis then
		// failing and a second goroutine acquiring the same key through the
		// local fallback, had its late Unlock release the SECOND goroutine's
		// local lock.
		//
		// ErrLockNotHeld is deliberately NOT in this list: it means this
		// locker has no Redis acquisition recorded for the key, which is
		// exactly what a lock taken through the local fallback looks like.
		if errors.Is(err, ErrLockValueMismatch) ||
			errors.Is(err, ErrLockValueType) ||
			errors.Is(err, ErrLockExpired) {
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

// storeEntry records an acquisition for key, oldest first.
//
// Only Lock and the package's own tests use it; callers identify a lock by
// key alone through the legacy API and have nothing to pass here.
func (r *RedisLocker) storeEntry(key string, entry lockEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue := append(r.lockStore[key], entry)
	if len(queue) > maxOutstandingPerKey {
		queue = queue[len(queue)-maxOutstandingPerKey:]
	}
	r.lockStore[key] = queue
}

// outstanding reports how many acquisitions are recorded for key.
func (r *RedisLocker) outstanding(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lockStore[key])
}

// trackedKeys reports how many keys have outstanding acquisitions recorded.
func (r *RedisLocker) trackedKeys() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lockStore)
}
