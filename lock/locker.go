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

	// operationMu serialises each legacy Lock/Unlock operation per key. Lock
	// keeps SET and enqueue in Redis execution order; Unlock keeps dequeue,
	// Eval and a possible restore indivisible so concurrent transport failures
	// cannot restore entries in reply-completion order and reorder the queue.
	operationMu keyedMutex

	// lockStore maps key -> the outstanding acquisitions for that key, for the
	// legacy Lock/Unlock pair -- which identifies a lock by key alone and so
	// has nowhere else to keep the token.
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
	lockStore map[string]*keyQueue
}

// keyQueue is the outstanding acquisitions for one key, oldest first.
type keyQueue struct {
	entries []lockEntry

	// tombstones counts acquisitions dropped by the cap.
	//
	// Their callers can still call Unlock, and simply discarding the entries
	// would shift every later caller onto somebody else's acquisition -- after
	// enough late unlocks a stale caller would reach the newest, still-live
	// token and compare-and-delete the current holder's lock. Each tombstone
	// absorbs exactly one Unlock, reporting ErrLockExpired, so the
	// caller-to-acquisition correspondence survives capping.
	tombstones int
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
		lockStore: make(map[string]*keyQueue),
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
	if r.lockTime <= 0 {
		return false, fmt.Errorf("lock time must be positive, got %s", r.lockTime)
	}

	lockValue, err := generateLockValue()
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()

	// The SET and the enqueue below are one operation per key. Apart, the
	// queue is ordered by REPLY COMPLETION rather than by Redis execution: A's
	// SET can succeed and have its reply delayed past the lease, B can then
	// acquire and reply first, and the queue records B before A. A's Unlock --
	// which takes the OLDEST entry -- would then pop B's live token and delete
	// a lock B is still holding.
	release := r.operationMu.lock(key)
	defer release()

	_, err = r.client.SetArgs(ctx, key, lockValue, redis.SetArgs{Mode: "NX", TTL: r.lockTime}).Result()
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("%w: failed to acquire lock: %w", ErrRedisUnavailable, err)
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

	q := r.lockStore[key]
	if q == nil {
		return lockEntry{}, false
	}

	var entry lockEntry
	switch {
	case q.tombstones > 0:
		// A capped acquisition: absorb this Unlock without touching Redis.
		// entry's zero expiry is already in the past, so the caller gets
		// ErrLockExpired.
		q.tombstones--
	case len(q.entries) > 0:
		entry = q.entries[0]
		q.entries = q.entries[1:]
	default:
		return lockEntry{}, false
	}

	// Balanced usage leaves nothing behind.
	if q.tombstones == 0 && len(q.entries) == 0 {
		delete(r.lockStore, key)
	}
	return entry, true
}

// Unlock releases a distributed lock using a Lua script to ensure atomicity.
// Only releases the lock if the lock value matches, preventing accidental
// release of another process's lock.
//
// It consumes the OLDEST outstanding acquisition of key -- and only on a
// DEFINITIVE result -- which is this caller's own in the paired usage this API
// assumes. The key alone cannot
// identify the caller, so when two holders of one key finish out of order --
// the second one's lease outliving the first's -- the second's Unlock takes
// the first's expired entry, reports ErrLockExpired, and leaves its own lock
// in place until the TTL elapses. That is the safe half of the trade: letting
// an Unlock skip an expired entry to find a live one is exactly how a late
// caller comes to delete a current holder's lock. Acquire returns a Handle
// carrying its own token and has neither problem.
func (r *RedisLocker) Unlock(key string) error {
	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	// Dequeue, Redis verdict and a possible restore are one operation per key.
	// Without this, two failed Unlock calls can restore their entries in the
	// opposite order: whichever Redis error returns last is prepended last.
	// A later retry can then consume the newer live token and release a lock it
	// did not acquire. The same mutex is used by Lock, so an acquisition cannot
	// be inserted into the queue halfway through this transaction either.
	release := r.operationMu.lock(key)
	defer release()

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
		// The entry goes BACK, at the front. ErrRedisUnavailable is not a
		// verdict about the lock, so consuming the acquisition on one would
		// point a retried Unlock at the NEXT entry: once this lease expired
		// and the same locker acquired the key again, that retry would
		// compare-and-delete the new, live lock. An acquisition is consumed
		// only by a definitive result.
		r.restoreOldest(key, entry)

		// Classified, not just wrapped. A transport failure is NOT a verdict
		// about the lock -- the lease may still be held -- and callers have to
		// be able to tell it apart from ErrLockExpired or a value mismatch.
		// HybridLocker's own guard tested for ErrRedisUnavailable here and
		// never saw it, so it treated every outage as definitive and dropped
		// the record that stops a local fallback being granted for a key
		// Redis may still hold.
		return fmt.Errorf("%w: failed to release lock: %w", ErrRedisUnavailable, err)
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

	// fallbackMu guards heldBy AND serialises the whole Lock/Unlock decision
	// while the fallback is enabled, so a key cannot be acquired through both
	// backends at once.
	fallbackMu sync.Mutex
	// held counts this locker's outstanding acquisitions per key, by backend.
	//
	// A COUNT, not a single marker: the legacy queue lets one locker hold the
	// same key more than once -- reacquiring after a lease elapses -- and a
	// key-level marker was cleared by the first definitive unlock. The newer
	// Redis lock was still live, so a local fallback could then be granted
	// alongside it.
	held map[string]*backendCounts
}

// backendCounts is how many acquisitions of one key are outstanding on each
// backend. At most one of the two is ever non-zero: Lock refuses to mix them.
type backendCounts struct {
	redis int
	local int
}

// fallbackActive reports whether this locker can hold a key through the local
// lock while also talking to Redis -- the only configuration in which an
// unlock has two possible destinations.
func (h *HybridLocker) fallbackActive() bool {
	return h.redisLocker != nil && h.allowLocalFallback
}

// counts returns key's outstanding acquisitions. Callers hold fallbackMu.
func (h *HybridLocker) counts(key string) backendCounts {
	if c := h.held[key]; c != nil {
		return *c
	}
	return backendCounts{}
}

// acquired records one more acquisition. Callers hold fallbackMu.
func (h *HybridLocker) acquired(key string, redisBacked bool) {
	if h.held == nil {
		h.held = make(map[string]*backendCounts)
	}
	c := h.held[key]
	if c == nil {
		c = &backendCounts{}
		h.held[key] = c
	}
	if redisBacked {
		c.redis++
	} else {
		c.local++
	}
}

// released records one fewer. Callers hold fallbackMu.
func (h *HybridLocker) released(key string, redisBacked bool) {
	c := h.held[key]
	if c == nil {
		return
	}
	if redisBacked {
		c.redis--
	} else {
		c.local--
	}
	if c.redis <= 0 && c.local <= 0 {
		delete(h.held, key)
	}
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
	if h.fallbackActive() {
		// Held for the whole decision, Redis call included. A key already
		// held through the local fallback must not be handed out again via
		// Redis the moment Redis recovers: that puts two holders in the
		// critical section, and the local holder's Unlock then goes on to
		// consume the Redis holder's token. Serialising costs throughput in a
		// mode that has already given up cross-instance exclusion.
		h.fallbackMu.Lock()
		defer h.fallbackMu.Unlock()

		switch c := h.counts(key); {
		case c.local > 0:
			// A local fallback is outstanding. Do not hand the key out
			// through Redis: that is a second holder in the same critical
			// section. The local locker refuses it, which is the answer.
			return h.localLocker.Lock(key)

		case c.redis > 0:
			// A Redis acquisition is outstanding. Never degrade to the local
			// lock for THIS key, even now that Redis is unreachable -- that
			// holder is still running. Refusing costs availability for one
			// key; degrading would put two callers inside it.
			success, err := h.redisLocker.Lock(key)
			if err == nil {
				if success {
					h.acquired(key, true)
				}
				return success, nil
			}
			return false, fmt.Errorf("%w: %w", ErrRedisUnavailable, err)

		default:
			success, err := h.redisLocker.Lock(key)
			if err == nil {
				if success {
					h.acquired(key, true)
				}
				return success, nil
			}
			// Explicitly opted in: mutual exclusion across instances is now
			// lost.
			success, err = h.localLocker.Lock(key)
			if success && err == nil {
				h.acquired(key, false)
			}
			return success, err
		}
	}

	if h.redisLocker != nil {
		success, err := h.redisLocker.Lock(key)
		if err == nil {
			return success, nil
		}
		return false, fmt.Errorf("%w: %w", ErrRedisUnavailable, err)
	}

	return h.localLocker.Lock(key)
}

// Unlock releases a lock through the backend that acquired it.
//
// The routing is recorded, not guessed. Trying Redis first and falling back to
// the local lock on error was a lock-stealing path in both directions: a
// locally-acquired key was released by deleting whatever Redis held for it --
// another goroutine's token, once Redis recovered -- and a Redis-acquired key
// whose release failed went on to release whatever was held locally.
func (h *HybridLocker) Unlock(key string) error {
	if h.fallbackActive() {
		h.fallbackMu.Lock()
		defer h.fallbackMu.Unlock()

		switch c := h.counts(key); {
		case c.local > 0:
			h.released(key, false)
			return h.localLocker.Unlock(key)

		case c.redis > 0:
			err := h.redisLocker.Unlock(key)
			// Drop ONE acquisition once the answer is definitive -- released,
			// or no longer ours. An unreachable Redis is not definitive: the
			// lease may still be held there, so the count stays and no local
			// fallback is granted for the key. RedisLocker.Unlock classifies
			// transport failures as ErrRedisUnavailable, which is what makes
			// this test mean anything.
			if err == nil || !errors.Is(err, ErrRedisUnavailable) {
				h.released(key, true)
			}
			return err

		default:
			// Nothing recorded: this locker never took the key. Ask Redis,
			// which will say so, and never reach for the local lock -- that
			// would release a lock this caller never held.
			return h.redisLocker.Unlock(key)
		}
	}

	if h.redisLocker != nil {
		return h.redisLocker.Unlock(key)
	}

	return h.localLocker.Unlock(key)
}

// storeEntry records an acquisition for key, oldest first.
//
// Only Lock and the package's own tests use it; callers identify a lock by
// key alone through the legacy API and have nothing to pass here.
// restoreOldest puts an acquisition back at the front of key's queue after an
// Unlock that got no verdict, so the retry consumes the same one.
func (r *RedisLocker) restoreOldest(key string, entry lockEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	q := r.lockStore[key]
	if q == nil {
		q = &keyQueue{}
		r.lockStore[key] = q
	}

	// At the FRONT: it is still the oldest outstanding acquisition, and any
	// entry appended while Redis was being asked is newer than it.
	q.entries = append([]lockEntry{entry}, q.entries...)
	if over := len(q.entries) - maxOutstandingPerKey; over > 0 {
		q.entries = q.entries[over:]
		q.tombstones += over
	}
}

func (r *RedisLocker) storeEntry(key string, entry lockEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	q := r.lockStore[key]
	if q == nil {
		q = &keyQueue{}
		r.lockStore[key] = q
	}

	q.entries = append(q.entries, entry)
	if over := len(q.entries) - maxOutstandingPerKey; over > 0 {
		// Leave a tombstone per dropped entry rather than silently shifting
		// later callers onto somebody else's acquisition.
		q.entries = q.entries[over:]
		q.tombstones += over
	}
}

// outstanding reports how many acquisitions are recorded for key.
func (r *RedisLocker) outstanding(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q := r.lockStore[key]; q != nil {
		return len(q.entries) + q.tombstones
	}
	return 0
}

// trackedKeys reports how many keys have outstanding acquisitions recorded.
func (r *RedisLocker) trackedKeys() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lockStore)
}

// keyedMutex serialises operations per key without serialising across keys.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

// lock blocks until key is free and returns the function that releases it.
// The per-key entry is dropped once nobody is waiting on it, so the map does
// not grow with the key space.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.m == nil {
		k.m = make(map[string]*keyedEntry)
	}
	e := k.m[key]
	if e == nil {
		e = &keyedEntry{}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(k.m, key)
		}
		k.mu.Unlock()
	}
}
