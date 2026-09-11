package lock

import (
	"context"
	"errors"
	"fmt"
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

// TestLockStoreDoesNotGrowUnbounded: the legacy store must not accumulate for
// the life of the process. A balanced Lock/Unlock pair leaves nothing behind,
// and an unbalanced one is capped.
func TestLockStoreDoesNotGrowUnbounded(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	// Balanced usage drops the key entirely.
	l := NewRedisLocker(client)
	for i := 0; i < 100; i++ {
		if _, err := l.Lock("balanced"); err != nil {
			t.Fatalf("Lock() error = %v", err)
		}
		if err := l.Unlock("balanced"); err != nil {
			t.Fatalf("Unlock() error = %v", err)
		}
	}
	if got := l.trackedKeys(); got != 0 {
		t.Errorf("lockStore tracks %d keys after balanced use, want 0", got)
	}

	// Acquiring without releasing is capped rather than unbounded.
	stale := NewRedisLockerWithLockTime(client, time.Nanosecond)
	for i := 0; i < maxOutstandingPerKey+50; i++ {
		if _, err := stale.Lock("leaky"); err != nil {
			t.Fatalf("Lock() error = %v", err)
		}
	}
	if got := stale.outstanding("leaky"); got > maxOutstandingPerKey {
		t.Errorf("lockStore holds %d entries for one key, want at most %d", got, maxOutstandingPerKey)
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

// --- Codex review follow-ups (PR #3) ---

// TestSameLockerReuseDoesNotStealALaterLock is the regression test for the
// legacy store keeping only the LATEST entry per key. Reusing one RedisLocker
// after a lease elapsed overwrote the first holder's token with the second's,
// so the first holder's Unlock compare-and-deleted the SECOND holder's lock --
// the exact stealing this store was added to prevent. The earlier regression
// test missed it by using two separate RedisLocker instances.
func TestSameLockerReuseDoesNotStealALaterLock(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	// One locker, short lease.
	l := NewRedisLockerWithLockTime(client, 30*time.Millisecond)
	const key = "shared"

	if ok, err := l.Lock(key); err != nil || !ok {
		t.Fatalf("first Lock = (%v, %v), want success", ok, err)
	}

	// The lease elapses and the Redis key goes with it.
	time.Sleep(60 * time.Millisecond)

	if ok, err := l.Lock(key); err != nil || !ok {
		t.Fatalf("second Lock = (%v, %v), want success after expiry", ok, err)
	}

	// The FIRST holder releases late. It must get its own expired entry back,
	// not the second holder's live one.
	if err := l.Unlock(key); !errors.Is(err, ErrLockExpired) {
		t.Errorf("stale Unlock() = %v, want ErrLockExpired", err)
	}

	// The second holder still owns its lock and can release it.
	if err := l.Unlock(key); err != nil {
		t.Errorf("second holder Unlock() = %v; its lock was released by the stale Unlock", err)
	}
}

// TestHandleExpiryIsMeasuredFromTheCommand is the regression test for stamping
// the deadline after the reply arrived. Redis starts the TTL when it executes
// SET, so measuring afterwards overstates the remaining lease by the whole
// round trip -- a caller scheduling renewal off ExpiresAt could still be
// running after another holder took the key.
func TestHandleExpiryIsMeasuredFromTheCommand(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)

	before := time.Now()
	h, err := l.Acquire(context.Background(), "timing", time.Second)
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v), want a handle", h, err)
	}
	after := time.Now()

	// The deadline must be stamped from before the command, never from after
	// the reply: erring early is the safe direction.
	if h.ExpiresAt().After(after.Add(time.Second)) {
		t.Errorf("ExpiresAt %s is later than reply+ttl %s; the lease is overstated",
			h.ExpiresAt(), after.Add(time.Second))
	}
	if h.ExpiresAt().Before(before.Add(time.Second)) {
		t.Errorf("ExpiresAt %s is earlier than issue+ttl %s", h.ExpiresAt(), before.Add(time.Second))
	}

	// Extend has the same rule.
	before = time.Now()
	if err := h.Extend(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	after = time.Now()
	if h.ExpiresAt().After(after.Add(2 * time.Second)) {
		t.Errorf("ExpiresAt %s after Extend is later than reply+ttl %s", h.ExpiresAt(), after.Add(2*time.Second))
	}
	if h.ExpiresAt().Before(before.Add(2 * time.Second)) {
		t.Errorf("ExpiresAt %s after Extend is earlier than issue+ttl %s", h.ExpiresAt(), before.Add(2*time.Second))
	}

	_ = h.Release(context.Background())
}

// TestExtendRoundsUpSubMillisecondTTLs: PEXPIRE takes whole milliseconds, so
// any positive TTL under 1ms truncated to 0 -- which DELETES the key while
// reporting success, and Extend returned nil for a lease it had just
// destroyed.
func TestExtendRoundsUpSubMillisecondTTLs(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	h, err := l.Acquire(context.Background(), "subms", time.Minute)
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v), want a handle", h, err)
	}

	if err := h.Extend(context.Background(), 100*time.Microsecond); err != nil {
		t.Fatalf("Extend(100µs) error = %v", err)
	}

	// The lock must still exist: a truncated PEXPIRE 0 would have deleted it
	// while Extend reported success.
	if err := h.Release(context.Background()); err != nil {
		t.Errorf("Release() after a sub-millisecond Extend = %v; the lock was destroyed by PEXPIRE 0", err)
	}
}

// --- Codex review round 2 (PR #3) ---

// TestCappedQueueKeepsTombstones is the regression test for bounding the
// legacy queue by discarding its oldest entries. Their callers can still call
// Unlock, so dropping the entries outright shifted every later caller onto
// somebody else's acquisition -- after enough late unlocks a stale caller
// reaches the newest, still-live token and compare-and-deletes the current
// holder's lock.
//
// The queue is driven directly: SET NX only succeeds once per key, so the
// overflow this guards against comes from repeated acquisitions across
// expired leases, not from a burst of concurrent Lock calls.
func TestCappedQueueKeepsTombstones(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	const key = "capped"
	const extra = 5

	for i := 0; i < maxOutstandingPerKey+extra; i++ {
		l.storeEntry(key, lockEntry{token: fmt.Sprintf("tok-%d", i), expires: time.Now().Add(time.Hour)})
	}

	// Every outstanding acquisition is still accounted for: entries plus
	// tombstones. Memory is capped; correspondence is not.
	if got := l.outstanding(key); got != maxOutstandingPerKey+extra {
		t.Fatalf("outstanding = %d, want %d: capping dropped acquisitions instead of tombstoning them",
			got, maxOutstandingPerKey+extra)
	}

	// The first `extra` unlocks belong to the discarded acquisitions and are
	// absorbed as expired, rather than consuming a later caller's entry.
	for i := 0; i < extra; i++ {
		if err := l.Unlock(key); !errors.Is(err, ErrLockExpired) {
			t.Fatalf("Unlock %d = %v, want ErrLockExpired from a tombstone", i, err)
		}
	}

	// The surviving entries are still there, in order, and the newest one --
	// the acquisition that would actually hold the Redis lock -- is reached
	// only by its own caller.
	if got := l.outstanding(key); got != maxOutstandingPerKey {
		t.Errorf("outstanding = %d after the tombstoned unlocks, want %d", got, maxOutstandingPerKey)
	}

	entry, ok := l.takeOldest(key)
	if !ok {
		t.Fatal("no entry left after the tombstones were consumed")
	}
	if entry.token != fmt.Sprintf("tok-%d", extra) {
		t.Errorf("oldest surviving token = %q, want tok-%d: the queue shifted", entry.token, extra)
	}
}

// TestExtendRoundsTTLUpToWholeMilliseconds: Redis TTLs are whole
// milliseconds and go-redis truncates, so a 1.9ms lease was sent as 1ms while
// the handle recorded 1.9ms -- overstating ownership by almost a millisecond.
func TestExtendRoundsTTLUpToWholeMilliseconds(t *testing.T) {
	if got, want := roundUpMillis(1900*time.Microsecond), 2*time.Millisecond; got != want {
		t.Errorf("roundUpMillis(1.9ms) = %s, want %s", got, want)
	}
	if got, want := roundUpMillis(100*time.Microsecond), time.Millisecond; got != want {
		t.Errorf("roundUpMillis(100µs) = %s, want %s", got, want)
	}
	if got, want := roundUpMillis(2*time.Millisecond), 2*time.Millisecond; got != want {
		t.Errorf("roundUpMillis(2ms) = %s, want %s (already whole)", got, want)
	}

	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	h, err := l.Acquire(context.Background(), "rounding", 1900*time.Microsecond)
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v), want a handle", h, err)
	}

	// The recorded deadline must not exceed what the server was told.
	if remaining := time.Until(h.ExpiresAt()); remaining > 2*time.Millisecond {
		t.Errorf("ExpiresAt is %s away, more than the rounded 2ms sent to Redis", remaining)
	}
	_ = h.Release(context.Background())
}

// TestHandleExpiryIsRaceFree exercises the renewal-heartbeat pattern the
// Handle API exists to support: Extend writing the deadline while another
// goroutine reads it through ExpiresAt. Run with -race.
func TestHandleExpiryIsRaceFree(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	l := NewRedisLocker(client)
	h, err := l.Acquire(context.Background(), "heartbeat", time.Minute)
	if err != nil || h == nil {
		t.Fatalf("Acquire = (%v, %v), want a handle", h, err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = h.Extend(context.Background(), time.Minute)
		}
	}()

	for i := 0; i < 200; i++ {
		_ = h.ExpiresAt()
	}
	<-done

	_ = h.Release(context.Background())
}
