package lock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soulteary/redis-kit/testutil"
)

func TestAcquireReleaseExtend(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	ctx := context.Background()

	h, err := l.Acquire(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if h == nil {
		t.Fatal("Acquire() = nil handle on a free key")
	}
	if h.Key() != "job" {
		t.Errorf("Key() = %q, want %q", h.Key(), "job")
	}

	// Contention is (nil, nil), not an error.
	busy, err := l.Acquire(ctx, "job", time.Minute)
	if err != nil {
		t.Errorf("Acquire() on a held key error = %v, want nil", err)
	}
	if busy != nil {
		t.Error("Acquire() on a held key returned a handle")
	}

	// Extending keeps the lease alive rather than losing it silently.
	if err := h.Extend(ctx, 2*time.Minute); err != nil {
		t.Errorf("Extend() error = %v", err)
	}
	if err := h.Extend(ctx, 0); err == nil {
		t.Error("Extend() with a non-positive ttl should fail")
	}

	if err := h.Release(ctx); err != nil {
		t.Errorf("Release() error = %v", err)
	}
	// Releasing twice reports it instead of deleting whatever is there now.
	if err := h.Release(ctx); err == nil {
		t.Error("second Release() should report the lock is no longer ours")
	}

	// The key is free again.
	again, err := l.Acquire(ctx, "job", time.Minute)
	if err != nil || again == nil {
		t.Fatalf("Acquire() after Release = (%v, %v), want a handle", again, err)
	}
}

// TestHandleReleaseDoesNotStealLaterLock is the regression test for the token
// aliasing bug: a stale holder must never release a lock somebody else took.
func TestHandleReleaseDoesNotStealLaterLock(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLockerWithLockTime(client, 50*time.Millisecond)
	ctx := context.Background()

	first, err := l.Acquire(ctx, "key", 50*time.Millisecond)
	if err != nil || first == nil {
		t.Fatalf("first Acquire = (%v, %v)", first, err)
	}

	time.Sleep(120 * time.Millisecond) // first lease elapses in Redis

	second, err := l.Acquire(ctx, "key", time.Minute)
	if err != nil || second == nil {
		t.Fatalf("second Acquire = (%v, %v), want a handle after the first expired", second, err)
	}

	// The first holder releasing late must not drop the second holder's lock.
	if err := first.Release(ctx); err == nil {
		t.Error("stale handle Release() succeeded; it released a lock it no longer held")
	}

	if err := second.Extend(ctx, time.Minute); err != nil {
		t.Errorf("second holder lost its lock to a stale Release: %v", err)
	}
	if err := second.Release(ctx); err != nil {
		t.Errorf("second holder Release() error = %v", err)
	}
}

// TestLegacyUnlockDoesNotStealLaterLock covers the same property for the
// key-only Lock/Unlock pair, which keeps the token in a process-wide map.
func TestLegacyUnlockDoesNotStealLaterLock(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	first := NewRedisLockerWithLockTime(client, 50*time.Millisecond)
	if ok, err := first.Lock("key"); err != nil || !ok {
		t.Fatalf("first Lock = (%v, %v)", ok, err)
	}

	time.Sleep(120 * time.Millisecond) // first lease elapses

	second := NewRedisLocker(client)
	if ok, err := second.Lock("key"); err != nil || !ok {
		t.Fatalf("second Lock = (%v, %v), want it to succeed after expiry", ok, err)
	}

	// The stale holder's Unlock is answered from its own recorded deadline,
	// without issuing a delete against a key it no longer owns.
	if err := first.Unlock("key"); !errors.Is(err, ErrLockExpired) {
		t.Errorf("stale Unlock() = %v, want ErrLockExpired", err)
	}
	if err := second.Unlock("key"); err != nil {
		t.Errorf("second holder Unlock() = %v; its lock was taken by the stale release", err)
	}
}

// TestLockStoreIsSwept: entries whose lease elapsed can no longer release
// anything, so they must not accumulate for the life of the process.
func TestLockStoreIsSwept(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLockerWithLockTime(client, time.Nanosecond)
	for i := 0; i < sweepInterval+1; i++ {
		if _, err := l.Lock(string(rune('a'+i%26)) + string(rune('a'+i/26))); err != nil {
			t.Fatalf("Lock() error = %v", err)
		}
	}

	remaining := 0
	l.lockStore.Range(func(_, _ any) bool { remaining++; return true })
	if remaining > sweepInterval {
		t.Errorf("lockStore holds %d expired entries after %d acquisitions; it is not being swept", remaining, sweepInterval+1)
	}
}

func TestAcquireNilClient(t *testing.T) {
	l := &RedisLocker{}
	h, err := l.Acquire(context.Background(), "k", time.Minute)
	if h != nil {
		t.Error("Acquire() with a nil client returned a handle")
	}
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("Acquire() error = %v, want ErrRedisUnavailable", err)
	}
}

func TestAcquireDefaultsTTL(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLockerWithLockTime(client, 30*time.Second)
	before := time.Now()

	h, err := l.Acquire(context.Background(), "ttl", 0) // 0 means "use the locker default"
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v)", h, err)
	}
	if got := h.ExpiresAt().Sub(before); got < 25*time.Second || got > 35*time.Second {
		t.Errorf("ExpiresAt is %s out, want roughly the locker's 30s default", got)
	}
}

func TestNilHandleOperations(t *testing.T) {
	var h *Handle
	if err := h.Release(context.Background()); !errors.Is(err, ErrLockNotHeld) {
		t.Errorf("(*Handle)(nil).Release() = %v, want ErrLockNotHeld", err)
	}
	if err := h.Extend(context.Background(), time.Minute); !errors.Is(err, ErrLockNotHeld) {
		t.Errorf("(*Handle)(nil).Extend() = %v, want ErrLockNotHeld", err)
	}
}

func TestHandleOperationsOnRedisFailure(t *testing.T) {
	client, mock := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	ctx := context.Background()

	h, err := l.Acquire(ctx, "fail", time.Minute)
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v)", h, err)
	}

	mock.SetShouldFail(true)
	defer mock.SetShouldFail(false)

	if _, err := l.Acquire(ctx, "other", time.Minute); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("Acquire() during a Redis failure = %v, want ErrRedisUnavailable", err)
	}
	if err := h.Extend(ctx, time.Minute); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("Extend() during a Redis failure = %v, want ErrRedisUnavailable", err)
	}
	if err := h.Release(ctx); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("Release() during a Redis failure = %v, want ErrRedisUnavailable", err)
	}
}
