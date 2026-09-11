package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// releaseScript deletes the key only if it still carries our token.
const releaseScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end
`

// extendScript refreshes the TTL only if the key still carries our token.
const extendScript = `
-- redis-kit:lock-extend
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
else
	return 0
end
`

// Handle is an acquired lock. It carries the token of the acquisition that
// produced it, so releasing it can never affect a lock somebody else acquired
// after this one's lease expired.
//
// This is the API to prefer over Locker.Lock/Unlock. Those identify a lock by
// key alone and keep the token in a process-wide map, which cannot distinguish
// two acquisitions of the same key.
type Handle struct {
	client  *redis.Client
	key     string
	token   string
	expires time.Time
}

// Key returns the locked key.
func (h *Handle) Key() string { return h.key }

// ExpiresAt returns when this lease elapses if it is not extended.
func (h *Handle) ExpiresAt() time.Time { return h.expires }

// Acquire takes the lock for key, holding it for ttl.
//
// It returns (nil, nil) when the lock is already held by someone else: that is
// contention, not an error. Any other non-nil error is a Redis failure and is
// wrapped in ErrRedisUnavailable, so callers can fail closed rather than guess.
func (l *RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (*Handle, error) {
	if l.client == nil {
		return nil, fmt.Errorf("%w: redis client is nil", ErrRedisUnavailable)
	}
	if ttl <= 0 {
		ttl = l.lockTime
	}

	token, err := generateLockValue()
	if err != nil {
		return nil, err
	}

	// Stamp the deadline from BEFORE the command is issued.
	//
	// Redis starts the TTL when it executes SET, not when the reply arrives.
	// Computing the deadline afterwards overstates the remaining lease by the
	// whole round trip: with a five-second lease and a four-second reply, the
	// handle claimed nearly five seconds while Redis had about one, so a
	// caller scheduling renewal off ExpiresAt could still be running after
	// another holder took the key. Erring early is the safe direction.
	issued := time.Now()

	err = l.client.SetArgs(ctx, key, token, redis.SetArgs{Mode: "NX", TTL: ttl}).Err()
	if errors.Is(err, redis.Nil) {
		return nil, nil // held by somebody else
	}
	if err != nil {
		return nil, fmt.Errorf("%w: acquire %q: %v", ErrRedisUnavailable, key, err)
	}

	return &Handle{client: l.client, key: key, token: token, expires: issued.Add(ttl)}, nil
}

// Release gives the lock back.
//
// It is a compare-and-delete on the token, so a Handle whose lease already
// elapsed cannot delete a lock a different holder has since taken. Releasing a
// lease that has already elapsed reports ErrLockExpired.
func (h *Handle) Release(ctx context.Context) error {
	if h == nil {
		return ErrLockNotHeld
	}

	res, err := h.client.Eval(ctx, releaseScript, []string{h.key}, h.token).Result()
	if err != nil {
		return fmt.Errorf("%w: release %q: %v", ErrRedisUnavailable, h.key, err)
	}
	if n, ok := res.(int64); !ok || n == 0 {
		if time.Now().After(h.expires) {
			return ErrLockExpired
		}
		return ErrLockValueMismatch
	}
	return nil
}

// Extend refreshes the lease by ttl, so work that outlives the original lease
// does not end up running alongside a second holder.
//
// Without this a lock is simply lost when the lease elapses, silently and with
// no error anywhere: the critical section keeps running while another instance
// acquires the same key.
func (h *Handle) Extend(ctx context.Context, ttl time.Duration) error {
	if h == nil {
		return ErrLockNotHeld
	}
	if ttl <= 0 {
		return fmt.Errorf("extend ttl must be positive, got %s", ttl)
	}

	// PEXPIRE takes whole milliseconds, and any positive TTL under 1ms
	// truncates to 0 -- which DELETES the key while reporting success, so
	// Extend returned nil for a lease it had just destroyed. Round up.
	millis := ttl.Milliseconds()
	if millis < 1 {
		millis = 1
		ttl = time.Millisecond
	}

	// Same pre-command stamp as Acquire: Redis restarts the TTL when it runs
	// PEXPIRE, so measuring from the reply overstates the new lease.
	issued := time.Now()

	res, err := h.client.Eval(ctx, extendScript, []string{h.key}, h.token, millis).Result()
	if err != nil {
		return fmt.Errorf("%w: extend %q: %v", ErrRedisUnavailable, h.key, err)
	}
	if n, ok := res.(int64); !ok || n == 0 {
		return ErrLockExpired
	}
	h.expires = issued.Add(ttl)
	return nil
}
