package lock

import "errors"

var (
	// ErrLockNotHeld indicates the lock was not held by this locker instance.
	ErrLockNotHeld = errors.New("lock not held")
	// ErrLockValueMismatch indicates the lock value doesn't match or lock expired.
	ErrLockValueMismatch = errors.New("lock value mismatch or lock has expired")
	// ErrLockValueType indicates the stored lock value has an unexpected type.
	ErrLockValueType = errors.New("lock value type error")
	// ErrLockExpired indicates this locker's conservative lease elapsed before
	// an acquisition/extension reply arrived or before Unlock was called, so
	// the lock may already be held by someone else. A late Unlock reports it
	// without touching Redis, precisely so it cannot delete a later holder's
	// key.
	ErrLockExpired = errors.New("lock lease expired before release")
	// ErrRedisUnavailable indicates a Redis operation failed. A HybridLocker
	// returns it rather than silently degrading to a process-local lock, which
	// would drop mutual exclusion across instances at the moment it matters.
	ErrRedisUnavailable = errors.New("redis unavailable")
)
