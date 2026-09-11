package lock

import "errors"

var (
	// ErrLockNotHeld indicates the lock was not held by this locker instance.
	ErrLockNotHeld = errors.New("lock not held")
	// ErrLockValueMismatch indicates the lock value doesn't match or lock expired.
	ErrLockValueMismatch = errors.New("lock value mismatch or lock has expired")
	// ErrLockValueType indicates the stored lock value has an unexpected type.
	ErrLockValueType = errors.New("lock value type error")
	// ErrLockExpired indicates this locker's lease elapsed before Unlock was
	// called, so the lock may already be held by someone else. It is reported
	// without touching Redis, precisely so a late Unlock cannot delete the key
	// a different holder has since acquired.
	ErrLockExpired = errors.New("lock lease expired before release")
	// ErrRedisUnavailable indicates a Redis operation failed. A HybridLocker
	// returns it rather than silently degrading to a process-local lock, which
	// would drop mutual exclusion across instances at the moment it matters.
	ErrRedisUnavailable = errors.New("redis unavailable")
)
