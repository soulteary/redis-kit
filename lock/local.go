package lock

import (
	"sync"
)

// LocalLocker provides local lock functionality using sync.Mutex
// Suitable for single-machine deployment scenarios, does not support distributed environments
type LocalLocker struct {
	mu    sync.Mutex
	locks map[string]bool
}

// NewLocalLocker creates a new local lock instance
func NewLocalLocker() *LocalLocker {
	return &LocalLocker{
		locks: make(map[string]bool),
	}
}

// Lock acquires a local lock
// Returns true if the lock was successfully acquired, false if the lock is already held
func (l *LocalLocker) Lock(key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// If lock is already held, return false
	if l.locks[key] {
		return false, nil
	}

	// Acquire lock
	l.locks[key] = true
	return true, nil
}

// Unlock releases a local lock
func (l *LocalLocker) Unlock(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Release lock
	delete(l.locks, key)
	return nil
}
