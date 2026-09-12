package lock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/redis-kit/testutil"
)

func TestNewRedisLocker(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	locker := NewRedisLocker(client)
	if locker == nil {
		t.Fatal("NewRedisLocker() returned nil")
	}
	if locker.client != client {
		t.Error("NewRedisLocker() client mismatch")
	}
	if locker.lockTime != DefaultLockTime {
		t.Errorf("NewRedisLocker() lockTime = %v, want %v", locker.lockTime, DefaultLockTime)
	}
}

func TestNewRedisLockerWithLockTime(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	customLockTime := 30 * time.Second
	locker := NewRedisLockerWithLockTime(client, customLockTime)
	if locker == nil {
		t.Fatal("NewRedisLockerWithLockTime() returned nil")
	}
	if locker.lockTime != customLockTime {
		t.Errorf("NewRedisLockerWithLockTime() lockTime = %v, want %v", locker.lockTime, customLockTime)
	}
}

func TestGenerateLockValue(t *testing.T) {
	// Test that generateLockValue creates unique values
	values := make(map[string]bool)
	for i := 0; i < 100; i++ {
		value, err := generateLockValue()
		if err != nil {
			t.Fatalf("generateLockValue() error = %v", err)
		}
		if values[value] {
			t.Errorf("generateLockValue() returned duplicate value: %s", value)
		}
		values[value] = true
		if len(value) == 0 {
			t.Error("generateLockValue() returned empty value")
		}
	}
}

func TestRedisLocker_Lock(t *testing.T) {
	t.Run("successful lock acquisition", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "test-lock"

		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("Lock() error = %v, want nil", err)
		}
		if !success {
			t.Error("Lock() = false, want true")
		}
	})

	t.Run("lock already held", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "test-lock"

		// Acquire lock first time
		success1, err1 := locker.Lock(key)
		if err1 != nil || !success1 {
			t.Fatal("First Lock() should succeed")
		}

		// Try to acquire same lock again (should fail)
		success2, err2 := locker.Lock(key)
		if err2 != nil {
			t.Errorf("Lock() error = %v, want nil", err2)
		}
		if success2 {
			t.Error("Lock() on already held lock = true, want false")
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		locker := &RedisLocker{
			client:   nil,
			lockTime: DefaultLockTime,
		}

		_, err := locker.Lock("test-key")
		if err == nil {
			t.Error("Lock() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Lock() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("different keys can be locked independently", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)

		success1, err1 := locker.Lock("key1")
		if err1 != nil || !success1 {
			t.Fatal("Lock(key1) should succeed")
		}

		success2, err2 := locker.Lock("key2")
		if err2 != nil {
			t.Errorf("Lock(key2) error = %v, want nil", err2)
		}
		if !success2 {
			t.Error("Lock(key2) = false, want true (different key should be lockable)")
		}

		// Clean up
		_ = locker.Unlock("key1")
		_ = locker.Unlock("key2")
	})

	t.Run("Redis operation failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		mock.SetShouldFail(true)

		_, err := locker.Lock("test-key")
		if err == nil {
			t.Error("Lock() with Redis failure should return error")
		}

		mock.SetShouldFail(false)
	})
}

func TestRedisLocker_Unlock(t *testing.T) {
	t.Run("successful unlock", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "test-lock"

		// Lock first
		_, _ = locker.Lock(key)

		// Unlock
		err := locker.Unlock(key)
		if err != nil {
			t.Errorf("Unlock() error = %v, want nil", err)
		}

		// Should be able to lock again
		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("Lock() after unlock error = %v, want nil", err)
		}
		if !success {
			t.Error("Lock() after unlock = false, want true")
		}
	})

	t.Run("lock value mismatch", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker1 := NewRedisLocker(client)
		locker2 := NewRedisLocker(client)
		key := "test-lock"

		// Lock with locker1
		_, _ = locker1.Lock(key)

		// Manually set a different lock value in locker2's lockStore to simulate mismatch
		// Then try to unlock - should fail because lock value doesn't match
		locker2.storeEntry(key, lockEntry{token: "wrong-value", expires: time.Now().Add(time.Minute)})

		// Try to unlock with locker2 (different lock value)
		err := locker2.Unlock(key)
		if err == nil {
			t.Error("Unlock() with mismatched lock value should return error")
		}
		if !errors.Is(err, ErrLockValueMismatch) {
			t.Errorf("Unlock() error = %v, want %v", err, ErrLockValueMismatch)
		}
	})

	t.Run("nil client error", func(t *testing.T) {
		locker := &RedisLocker{
			client:   nil,
			lockTime: DefaultLockTime,
		}

		err := locker.Unlock("test-key")
		if err == nil {
			t.Error("Unlock() with nil client should return error")
		}
		if err.Error() != "redis client is nil" {
			t.Errorf("Unlock() error = %q, want %q", err.Error(), "redis client is nil")
		}
	})

	t.Run("unlock non-existent lock (backward compatibility)", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "non-existent-lock"

		// Unlocking a non-existent lock should error
		err := locker.Unlock(key)
		if !errors.Is(err, ErrLockNotHeld) {
			t.Errorf("Unlock() on non-existent lock error = %v, want %v", err, ErrLockNotHeld)
		}
	})

	t.Run("unlock expired lock", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLockerWithLockTime(client, 50*time.Millisecond)
		key := "expired-lock"

		// Lock
		_, _ = locker.Lock(key)

		// Wait for lock to expire
		time.Sleep(100 * time.Millisecond)

		// Try to unlock expired lock
		err := locker.Unlock(key)
		if err == nil {
			t.Log("Unlock() on expired lock succeeded (lock may have been auto-expired)")
		}
		// An elapsed lease is reported as ErrLockExpired and, crucially, without
		// issuing the delete: the key may already belong to a later holder.
		if err != nil && !errors.Is(err, ErrLockExpired) && !errors.Is(err, ErrLockValueMismatch) && !errors.Is(err, ErrLockNotHeld) {
			t.Errorf("Unlock() on expired lock error = %v, want expired, mismatch or not held", err)
		}
	})

	t.Run("unlock with Redis operation failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "test-lock"

		// Lock first
		_, _ = locker.Lock(key)

		// Make Redis fail
		mock.SetShouldFail(true)

		// Try to unlock (should fail)
		err := locker.Unlock(key)
		if err == nil {
			t.Error("Unlock() with Redis failure should return error")
		}

		// Reset
		mock.SetShouldFail(false)
	})

	t.Run("unlock with lock value type error", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "test-lock"

		// lockStore is now a typed []lockEntry, so a wrongly-typed entry
		// cannot be represented and Unlock can no longer return
		// ErrLockValueType. The sentinel stays exported, and HybridLocker
		// still refuses to fall back on it, for callers matching on it.
		_, _ = locker.Lock(key)
		if err := locker.Unlock(key); err != nil {
			t.Errorf("Unlock() error = %v", err)
		}
	})
}

func TestHybridLocker(t *testing.T) {
	t.Run("creates hybrid locker with Redis", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		if locker == nil {
			t.Fatal("NewHybridLocker() returned nil")
		}
		if locker.redisLocker == nil {
			t.Error("NewHybridLocker() redisLocker is nil")
		}
		if locker.localLocker == nil {
			t.Error("NewHybridLocker() localLocker is nil")
		}
	})

	t.Run("creates hybrid locker without Redis", func(t *testing.T) {
		locker := NewHybridLocker(nil)
		if locker == nil {
			t.Fatal("NewHybridLocker() returned nil")
		}
		if locker.redisLocker != nil {
			t.Error("NewHybridLocker() with nil client should have nil redisLocker")
		}
		if locker.localLocker == nil {
			t.Error("NewHybridLocker() localLocker is nil")
		}
	})

	t.Run("uses Redis lock when available", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		key := "test-lock"

		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("HybridLocker.Lock() error = %v, want nil", err)
		}
		if !success {
			t.Error("HybridLocker.Lock() = false, want true")
		}

		// Should be able to unlock
		err = locker.Unlock(key)
		if err != nil {
			t.Errorf("HybridLocker.Unlock() error = %v, want nil", err)
		}
	})

	// A Redis outage must not be answered with a process-local lock by default.
	// Doing so drops mutual exclusion between instances while reporting
	// (true, nil), which is indistinguishable from a real distributed lock.
	t.Run("reports the Redis failure instead of degrading to a local lock", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		key := "test-lock"

		success, err := locker.Lock(key)
		if err == nil {
			t.Error("HybridLocker.Lock() with failed Redis returned nil error; the caller cannot fail closed")
		}
		if !errors.Is(err, ErrRedisUnavailable) {
			t.Errorf("HybridLocker.Lock() error = %v, want ErrRedisUnavailable", err)
		}
		if success {
			t.Error("HybridLocker.Lock() with failed Redis = true; no lock was actually taken anywhere")
		}
	})

	// The previous behaviour stays available for callers that genuinely accept
	// losing exclusion, but it has to be asked for by name.
	t.Run("opt-in local fallback still works", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer func() { _ = client.Close() }()

		locker := NewHybridLockerWithLocalFallback(client)
		key := "test-lock"

		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("opt-in fallback Lock() error = %v, want nil", err)
		}
		if !success {
			t.Error("opt-in fallback Lock() = false, want true")
		}
		if err := locker.Unlock(key); err != nil {
			t.Errorf("opt-in fallback Unlock() error = %v, want nil", err)
		}
	})

	t.Run("uses local lock when Redis client is nil", func(t *testing.T) {
		locker := NewHybridLocker(nil)
		key := "test-lock"

		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("HybridLocker.Lock() with nil Redis error = %v, want nil", err)
		}
		if !success {
			t.Error("HybridLocker.Lock() with nil Redis = false, want true")
		}

		err = locker.Unlock(key)
		if err != nil {
			t.Errorf("HybridLocker.Unlock() error = %v, want nil", err)
		}
	})

	t.Run("concurrent hybrid lock", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		key := "concurrent-lock"

		// First lock should succeed
		success1, err1 := locker.Lock(key)
		if err1 != nil || !success1 {
			t.Fatal("First Lock() should succeed")
		}

		// Second lock should fail (Redis lock is held)
		success2, err2 := locker.Lock(key)
		if err2 != nil {
			t.Errorf("Second Lock() error = %v, want nil", err2)
		}
		if success2 {
			t.Error("Second Lock() = true, want false (lock already held)")
		}

		// Unlock and try again
		_ = locker.Unlock(key)
		success3, err3 := locker.Lock(key)
		if err3 != nil || !success3 {
			t.Error("Lock() after unlock should succeed")
		}

		_ = locker.Unlock(key)
	})

	t.Run("hybrid unlock with Redis error falls back to local", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLockerWithLocalFallback(client)
		key := "test-lock"

		// Lock via local (by making Redis fail)
		mock.SetShouldFail(true)
		_, _ = locker.Lock(key)
		mock.SetShouldFail(false)

		// Unlock should work via local
		err := locker.Unlock(key)
		if err != nil {
			t.Errorf("HybridLocker.Unlock() with local lock error = %v, want nil", err)
		}
	})

	t.Run("hybrid unlock returns error on lock value mismatch without fallback", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		key := "mismatch-lock"

		// Lock with Redis
		_, _ = locker.Lock(key)

		// Simulate another locker instance trying to unlock (wrong value in store)
		locker2 := NewRedisLocker(client)
		locker2.storeEntry(key, lockEntry{token: "wrong-value", expires: time.Now().Add(time.Minute)})
		// Unlock with locker2 fails with "lock value mismatch or lock has expired"
		err2 := locker2.Unlock(key)
		if err2 == nil {
			t.Fatal("locker2.Unlock() should fail with mismatch")
		}

		// Hybrid: unlock with a hybrid that has Redis locker but "our" unlock would get mismatch
		// We need Hybrid to get the mismatch path: Redis unlock returns error containing "lock value mismatch"
		// So use the same hybrid locker but corrupt its redis lockStore for this key so Redis Unlock returns mismatch
		hl := NewHybridLocker(client)
		_, _ = hl.Lock(key) // now hl holds the lock
		// Corrupt: make Redis think we have different value so Eval returns 0
		hl.redisLocker.storeEntry(key, lockEntry{token: "wrong-value", expires: time.Now().Add(time.Minute)})
		err := hl.Unlock(key)
		// Should return the mismatch error, not fall back to local
		if err == nil {
			t.Error("HybridLocker.Unlock() with lock value mismatch should return error")
		}
		if !errors.Is(err, ErrLockValueMismatch) {
			t.Errorf("HybridLocker.Unlock() error = %v, want %v", err, ErrLockValueMismatch)
		}
	})

	// Redis Unlock 失败（如网络错误）且本地未持有该 key 时，应返回 Redis 错误而非 fallback 成功
	t.Run("hybrid unlock returns Redis error when Redis fails and local lock not held", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewHybridLocker(client)
		key := "redis-fail-key"

		// 先通过 Redis 加锁成功
		success, err := locker.Lock(key)
		if err != nil || !success {
			t.Fatalf("Lock() = %v, %v, want true, nil", success, err)
		}

		// 模拟 Unlock 时 Redis 报错（如连接失败）
		mock.SetShouldFail(true)
		defer mock.SetShouldFail(false)

		// Unlock：Redis 会失败，且该 key 是 Redis 锁持有的，本地 locker 没有此 key，fallback 到 local unlock 会返回 ErrLockNotHeld
		// 因此应返回 Redis 的错误，而不是 nil
		err = locker.Unlock(key)
		if err == nil {
			t.Error("HybridLocker.Unlock() when Redis fails and local not held should return error")
		}
	})
}

func TestRedisLocker_Concurrent(t *testing.T) {
	t.Run("concurrent lock contention", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		locker := NewRedisLocker(client)
		key := "concurrent-key"
		successCount := 0
		var mu sync.Mutex
		var wg sync.WaitGroup
		numGoroutines := 10

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				success, err := locker.Lock(key)
				if err != nil {
					t.Errorf("Lock() error = %v", err)
					return
				}
				if success {
					mu.Lock()
					successCount++
					mu.Unlock()

					// Hold lock briefly
					time.Sleep(10 * time.Millisecond)

					// Unlock
					if err := locker.Unlock(key); err != nil {
						t.Errorf("Unlock() error = %v", err)
					}
				}
			}()
		}

		wg.Wait()

		// Only one goroutine should have successfully acquired the lock
		if successCount != 1 {
			t.Errorf("concurrent Lock() successCount = %d, want 1", successCount)
		}
	})
}

// TestFallbackKeyIsNotReissuedThroughRedis is the regression test for a local
// fallback acquisition being invisible to the Redis path.
//
// During an outage NewHybridLockerWithLocalFallback takes the key locally.
// Once Redis recovers, a second Lock of the SAME key went to Redis and
// succeeded -- two holders in one critical section -- and the first holder's
// Unlock then tried Redis first and deleted the second holder's token while
// leaving its own local lock behind.
func TestFallbackKeyIsNotReissuedThroughRedis(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	// A client that cannot connect: the acquisition below degrades to local.
	dead := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer func() { _ = dead.Close() }()

	h := NewHybridLockerWithLocalFallback(dead)

	ok, err := h.Lock("k")
	if err != nil || !ok {
		t.Fatalf("Lock during the outage = (%v, %v), want (true, nil) from the local fallback", ok, err)
	}

	// Redis comes back.
	h.redisLocker = NewRedisLocker(client)

	if ok, _ := h.Lock("k"); ok {
		t.Error("a key held through the local fallback was handed out again via Redis")
	}

	// Someone else takes the key in Redis. The fallback holder's Unlock must
	// not touch it.
	other := NewRedisLocker(client)
	if ok, err := other.Lock("k"); err != nil || !ok {
		t.Fatalf("the Redis holder could not take the key: (%v, %v)", ok, err)
	}

	if err := h.Unlock("k"); err != nil {
		t.Errorf("Unlock of the fallback acquisition = %v, want nil", err)
	}
	if ok, _ := other.Lock("k"); ok {
		t.Error("the fallback holder's Unlock deleted the Redis holder's lock")
	}
}

// TestRedisHeldKeyIsNotGrantedLocally is the regression test for the REVERSE
// transition of the round-3 fix: a key taken through Redis, with Redis then
// becoming unavailable before it is released.
//
// The second Lock found no fallback record, got a Redis error, and took the
// local lock -- two callers in one critical section. Worse, the record is
// keyed by KEY, so the Redis holder's own Unlock then found the local marker
// and released the SECOND caller's lock.
func TestRedisHeldKeyIsNotGrantedLocally(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	h := NewHybridLockerWithLocalFallback(client)

	// Taken through a healthy Redis.
	ok, err := h.Lock("k")
	if err != nil || !ok {
		t.Fatalf("Lock through Redis = (%v, %v), want (true, nil)", ok, err)
	}

	// Redis goes away while that lease is still held.
	dead := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer func() { _ = dead.Close() }()
	h.redisLocker = NewRedisLocker(dead)

	if ok, _ := h.Lock("k"); ok {
		t.Error("a key held through Redis was granted again through the local fallback")
	}

	// A DIFFERENT key may still degrade -- that is what the mode is for.
	if ok, err := h.Lock("other"); err != nil || !ok {
		t.Errorf("Lock of an unheld key during the outage = (%v, %v), want the fallback to grant it", ok, err)
	}
}

// TestFallbackRoutingIsSerializedPerKey is the regression test for holding a
// single mutex across every Redis call in fallback mode. One slow key made all
// unrelated keys queue behind its operation timeout, while the routing
// invariant only requires decisions for the same key to be indivisible.
func TestFallbackRoutingIsSerializedPerKey(t *testing.T) {
	t.Run("different keys proceed independently", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		hook := newDelayReplyHook("set", "")
		client.AddHook(hook)
		locker := NewHybridLockerWithLocalFallback(client)
		type result struct {
			ok  bool
			err error
		}
		firstDone := make(chan result, 1)
		go func() {
			ok, err := locker.Lock("slow-key")
			firstDone <- result{ok: ok, err: err}
		}()

		select {
		case <-hook.processed:
		case <-time.After(2 * time.Second):
			close(hook.unblock)
			t.Fatal("first key did not reach the delayed-reply window")
		}

		secondDone := make(chan result, 1)
		go func() {
			ok, err := locker.Lock("independent-key")
			secondDone <- result{ok: ok, err: err}
		}()

		var second result
		blocked := false
		select {
		case second = <-secondDone:
		case <-time.After(250 * time.Millisecond):
			blocked = true
		}
		close(hook.unblock)
		first := <-firstDone
		if blocked {
			second = <-secondDone
			t.Fatal("an unrelated key was blocked by the first key's Redis reply")
		}
		if !first.ok || first.err != nil {
			t.Errorf("first Lock = (%v, %v), want (true, nil)", first.ok, first.err)
		}
		if !second.ok || second.err != nil {
			t.Errorf("second Lock = (%v, %v), want (true, nil)", second.ok, second.err)
		}
	})

	t.Run("same key remains indivisible", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer func() { _ = client.Close() }()

		hook := newDelayReplyHook("set", "")
		client.AddHook(hook)
		locker := NewHybridLockerWithLocalFallback(client)
		type result struct {
			ok  bool
			err error
		}
		firstDone := make(chan result, 1)
		go func() {
			ok, err := locker.Lock("same-key")
			firstDone <- result{ok: ok, err: err}
		}()

		select {
		case <-hook.processed:
		case <-time.After(2 * time.Second):
			close(hook.unblock)
			t.Fatal("first acquisition did not reach the delayed-reply window")
		}
		// Redis is lost after accepting the first lock but before its caller
		// records the backend. A concurrent same-key decision must wait for that
		// record rather than granting a local fallback.
		mock.SetShouldFail(true)
		secondDone := make(chan result, 1)
		go func() {
			ok, err := locker.Lock("same-key")
			secondDone <- result{ok: ok, err: err}
		}()

		returnedEarly := false
		select {
		case <-secondDone:
			returnedEarly = true
		case <-time.After(100 * time.Millisecond):
		}
		close(hook.unblock)
		first := <-firstDone
		if returnedEarly {
			t.Fatal("a same-key routing decision completed before the first was recorded")
		}
		second := <-secondDone
		if !first.ok || first.err != nil {
			t.Errorf("first Lock = (%v, %v), want (true, nil)", first.ok, first.err)
		}
		if second.ok || !errors.Is(second.err, ErrRedisUnavailable) {
			t.Errorf("same-key Lock during outage = (%v, %v), want refusal with ErrRedisUnavailable", second.ok, second.err)
		}
	})
}

// TestFallbackPreservesLateLeaseCleanupFailure covers the dual-classified
// error from a successful SET that arrived too late and whose token cleanup
// then failed. It matches ErrLockExpired and ErrRedisUnavailable; expiration
// must win so Hybrid never hides the uncertain Redis lock with a local grant.
func TestFallbackPreservesLateLeaseCleanupFailure(t *testing.T) {
	client, mock := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	hook := newCommandQueueDelayHook("set", "")
	hook.after = func() { mock.SetShouldFail(true) }
	client.AddHook(hook)
	locker := NewHybridLockerWithLocalFallback(client)
	locker.redisLocker.lockTime = 100 * time.Millisecond

	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, err := locker.Lock("late-cleanup")
		done <- result{ok: ok, err: err}
	}()

	select {
	case <-hook.processed:
	case <-time.After(2 * time.Second):
		close(hook.unblock)
		t.Fatal("Lock did not reach the queued-command window")
	}
	time.Sleep(120 * time.Millisecond)
	close(hook.unblock)
	got := <-done
	if got.ok || !errors.Is(got.err, ErrLockExpired) || !errors.Is(got.err, ErrRedisUnavailable) {
		t.Errorf("Hybrid Lock after failed late cleanup = (%v, %v), want false and both sentinels", got.ok, got.err)
	}

	// A hidden fallback would already occupy the local key.
	if ok, err := locker.localLocker.Lock("late-cleanup"); err != nil || !ok {
		t.Errorf("local key was occupied after rejected fallback: (%v, %v)", ok, err)
	}
}

// TestLegacyQueueFollowsRedisOrder is the regression test for enqueueing
// outside the SET. Apart, the queue is ordered by reply completion rather than
// by Redis execution, so a delayed reply could file the LATER acquisition
// first and the earlier caller's Unlock -- which takes the oldest entry --
// would pop the live token of the one still working.
func TestLegacyQueueFollowsRedisOrder(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	locker := NewRedisLocker(client)
	locker.lockTime = 50 * time.Millisecond

	// Two acquisitions of one key straddling the lease, concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 40; attempt++ {
				if ok, err := locker.Lock("k"); err == nil && ok {
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}
	wg.Wait()

	// However the replies interleaved, every recorded entry must be in
	// non-decreasing expiry order: that is what makes "oldest first" mean
	// "the earliest acquisition".
	locker.mu.Lock()
	q := locker.lockStore["k"]
	locker.mu.Unlock()
	if q == nil || len(q.entries) < 2 {
		t.Skipf("only %d acquisitions recorded; the lease did not elapse between them", len(q.entries))
	}
	for i := 1; i < len(q.entries); i++ {
		if q.entries[i].expires.Before(q.entries[i-1].expires) {
			t.Errorf("entry %d expires before entry %d; the queue is not in acquisition order", i, i-1)
		}
	}
}

// TestUnlockTransportFailureIsClassified is the regression test for
// RedisLocker.Unlock wrapping the raw redis error. HybridLocker's guard tested
// for ErrRedisUnavailable there and never saw it, so every outage looked
// definitive and the guard that stops a local fallback being granted for a key
// Redis may still hold was inert.
func TestUnlockTransportFailureIsClassified(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	locker := NewRedisLocker(client)

	ok, err := locker.Lock("k")
	if err != nil || !ok {
		t.Fatalf("Lock = (%v, %v), want (true, nil)", ok, err)
	}

	// The connection dies while the lease is still held.
	_ = client.Close()

	err = locker.Unlock("k")
	if err == nil {
		t.Fatal("Unlock on a dead connection returned nil")
	}
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("Unlock error = %v, want it to wrap ErrRedisUnavailable", err)
	}
	// And it must NOT look like a verdict about the lock.
	if errors.Is(err, ErrLockExpired) || errors.Is(err, ErrLockValueMismatch) {
		t.Errorf("Unlock error = %v, want no verdict about the lock", err)
	}
}

// TestFallbackWaitsForEveryRedisAcquisition is the regression test for the
// key-level backend marker. One locker can hold the same key more than once --
// the legacy queue allows reacquiring after a lease elapses -- and the first
// definitive unlock cleared the marker while the newer lock was still live, so
// a local fallback could be granted alongside it.
func TestFallbackWaitsForEveryRedisAcquisition(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	h := NewHybridLockerWithLocalFallback(client)
	h.redisLocker.lockTime = 40 * time.Millisecond

	// First acquisition.
	if ok, err := h.Lock("k"); err != nil || !ok {
		t.Fatalf("first Lock = (%v, %v), want (true, nil)", ok, err)
	}
	// Its lease elapses and the key is taken again by this same locker.
	time.Sleep(60 * time.Millisecond)
	if ok, err := h.Lock("k"); err != nil || !ok {
		t.Fatalf("second Lock = (%v, %v), want (true, nil) after the lease elapsed", ok, err)
	}

	// The FIRST holder finally unlocks. Its lease is gone, so this is
	// definitive -- but the second acquisition is still outstanding.
	if err := h.Unlock("k"); !errors.Is(err, ErrLockExpired) {
		t.Fatalf("first Unlock = %v, want ErrLockExpired", err)
	}

	// Redis now goes away. The still-live second acquisition must keep the
	// key off the local fallback.
	dead := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer func() { _ = dead.Close() }()
	h.redisLocker = NewRedisLocker(dead)

	if ok, _ := h.Lock("k"); ok {
		t.Error("a key with a live Redis acquisition outstanding was granted through the local fallback")
	}
}

// --- Codex review round 6 (PR #3) ---

// TestAmbiguousUnlockKeepsItsAcquisition is the regression test for consuming
// the queue entry on a transport failure.
//
// ErrRedisUnavailable is explicitly NOT a verdict about the lock, but Unlock
// had already popped the acquisition before asking Redis, so the entry was
// gone either way. Once that lease expired and the same locker took the key
// again, retrying the original Unlock reached for the NEXT entry and
// compare-and-deleted a lock that was still live -- the lock-stealing this
// queue exists to prevent, through the one path that looks like a retry.
func TestAmbiguousUnlockKeepsItsAcquisition(t *testing.T) {
	client, mock := testutil.NewMockRedisClient()
	defer func() { _ = client.Close() }()

	locker := NewRedisLockerWithLockTime(client, 60*time.Millisecond)

	if ok, err := locker.Lock("k"); err != nil || !ok {
		t.Fatalf("first Lock = (%v, %v), want (true, nil)", ok, err)
	}

	// Redis fails while the lease is still held: no verdict either way.
	mock.SetShouldFail(true)
	if err := locker.Unlock("k"); !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("Unlock during an outage = %v, want ErrRedisUnavailable", err)
	}
	mock.SetShouldFail(false)

	// The lease elapses and the SAME locker takes the key again.
	time.Sleep(90 * time.Millisecond)
	if ok, err := locker.Lock("k"); err != nil || !ok {
		t.Fatalf("second Lock = (%v, %v), want (true, nil) after the lease elapsed", ok, err)
	}

	locker.mu.Lock()
	q := locker.lockStore["k"]
	if q == nil || len(q.entries) == 0 {
		locker.mu.Unlock()
		t.Fatal("the second acquisition was not recorded")
	}
	liveToken := q.entries[len(q.entries)-1].token
	locker.mu.Unlock()

	// The caller retries the Unlock that got no verdict. It must consume ITS
	// OWN acquisition -- long expired -- and never reach Redis.
	if err := locker.Unlock("k"); !errors.Is(err, ErrLockExpired) {
		t.Errorf("retried Unlock = %v, want ErrLockExpired for its own elapsed lease", err)
	}

	got, err := client.Get(context.Background(), "k").Result()
	if err != nil {
		t.Fatalf("the live second lock was deleted by the retried Unlock: %v", err)
	}
	if got != liveToken {
		t.Errorf("Redis holds %q, want the second acquisition's token %q", got, liveToken)
	}
}
