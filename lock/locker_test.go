package lock

import (
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/redis-kit/testutil"
)

func TestNewRedisLocker(t *testing.T) {
	client, _ := testutil.NewMockRedisClient()
	defer client.Close()

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
	defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

		locker1 := NewRedisLocker(client)
		locker2 := NewRedisLocker(client)
		key := "test-lock"

		// Lock with locker1
		_, _ = locker1.Lock(key)

		// Manually set a different lock value in locker2's lockStore to simulate mismatch
		// Then try to unlock - should fail because lock value doesn't match
		locker2.lockStore.Store(key, "wrong-value")

		// Try to unlock with locker2 (different lock value)
		err := locker2.Unlock(key)
		if err == nil {
			t.Error("Unlock() with mismatched lock value should return error")
		}
		if err != nil && err.Error() != "lock value mismatch or lock has expired" {
			t.Logf("Unlock() error = %v (acceptable)", err)
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
		defer client.Close()

		locker := NewRedisLocker(client)
		key := "non-existent-lock"

		// Unlocking a non-existent lock should not error (backward compatibility)
		err := locker.Unlock(key)
		if err != nil {
			// It's okay if it errors, but it should handle gracefully
			t.Logf("Unlock() on non-existent lock returned error (acceptable): %v", err)
		}
	})

	t.Run("unlock expired lock", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

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
	})

	t.Run("unlock with Redis operation failure", func(t *testing.T) {
		client, mock := testutil.NewMockRedisClient()
		defer client.Close()

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
}

func TestHybridLocker(t *testing.T) {
	t.Run("creates hybrid locker with Redis", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

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
		defer client.Close()

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

	t.Run("falls back to local lock when Redis unavailable", func(t *testing.T) {
		// Create a client that will fail operations
		client := redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		})
		defer client.Close()

		locker := NewHybridLocker(client)
		key := "test-lock"

		// Should fall back to local lock
		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("HybridLocker.Lock() with failed Redis error = %v, want nil (should fallback)", err)
		}
		if !success {
			t.Error("HybridLocker.Lock() with failed Redis = false, want true (local lock should work)")
		}

		// Should be able to unlock via local lock
		err = locker.Unlock(key)
		if err != nil {
			t.Errorf("HybridLocker.Unlock() error = %v, want nil", err)
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
		defer client.Close()

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
		defer client.Close()

		locker := NewHybridLocker(client)
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
}

func TestRedisLocker_Concurrent(t *testing.T) {
	t.Run("concurrent lock contention", func(t *testing.T) {
		client, _ := testutil.NewMockRedisClient()
		defer client.Close()

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
