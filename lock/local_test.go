package lock

import (
	"sync"
	"testing"
	"time"
)

func TestNewLocalLocker(t *testing.T) {
	locker := NewLocalLocker()
	if locker == nil {
		t.Fatal("NewLocalLocker() returned nil")
	}
	if locker.locks == nil {
		t.Error("NewLocalLocker() locks map is nil")
	}
}

func TestLocalLocker_Lock(t *testing.T) {
	t.Run("successful lock acquisition", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "test-key"

		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("Lock() error = %v, want nil", err)
		}
		if !success {
			t.Error("Lock() = false, want true")
		}
	})

	t.Run("lock already held", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "test-key"

		// Acquire lock first time
		success1, err1 := locker.Lock(key)
		if err1 != nil || !success1 {
			t.Fatal("First Lock() should succeed")
		}

		// Try to acquire same lock again
		success2, err2 := locker.Lock(key)
		if err2 != nil {
			t.Errorf("Lock() error = %v, want nil", err2)
		}
		if success2 {
			t.Error("Lock() on already held lock = true, want false")
		}
	})

	t.Run("different keys can be locked independently", func(t *testing.T) {
		locker := NewLocalLocker()

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
	})
}

func TestLocalLocker_Unlock(t *testing.T) {
	t.Run("successful unlock", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "test-key"

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

	t.Run("unlock non-existent lock", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "non-existent-key"

		// Unlocking a non-existent lock should succeed (no error)
		err := locker.Unlock(key)
		if err != nil {
			t.Errorf("Unlock() on non-existent lock error = %v, want nil", err)
		}
	})

	t.Run("unlock allows re-locking", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "test-key"

		// Lock and unlock
		_, _ = locker.Lock(key)
		_ = locker.Unlock(key)

		// Should be able to lock again
		success, err := locker.Lock(key)
		if err != nil {
			t.Errorf("Lock() after unlock error = %v, want nil", err)
		}
		if !success {
			t.Error("Lock() after unlock = false, want true")
		}
	})
}

func TestLocalLocker_Concurrent(t *testing.T) {
	t.Run("concurrent lock contention", func(t *testing.T) {
		locker := NewLocalLocker()
		key := "concurrent-key"
		numGoroutines := 10
		successCount := 0
		var mu sync.Mutex
		var wg sync.WaitGroup

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

	t.Run("concurrent different keys", func(t *testing.T) {
		locker := NewLocalLocker()
		numKeys := 5
		numGoroutinesPerKey := 3
		var wg sync.WaitGroup

		for i := 0; i < numKeys; i++ {
			key := "key-" + string(rune('a'+i))
			for j := 0; j < numGoroutinesPerKey; j++ {
				wg.Add(1)
				go func(k string) {
					defer wg.Done()
					success, err := locker.Lock(k)
					if err != nil {
						t.Errorf("Lock(%s) error = %v", k, err)
						return
					}
					if success {
						time.Sleep(5 * time.Millisecond)
						_ = locker.Unlock(k)
					}
				}(key)
			}
		}

		wg.Wait()

		// All keys should be unlockable now
		for i := 0; i < numKeys; i++ {
			key := "key-" + string(rune('a'+i))
			success, err := locker.Lock(key)
			if err != nil {
				t.Errorf("Lock(%s) error = %v", key, err)
			}
			if !success {
				t.Errorf("Lock(%s) = false, want true (should be unlockable)", key)
			}
			_ = locker.Unlock(key)
		}
	})
}
