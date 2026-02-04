package lock

import "errors"

var (
	// ErrLockNotHeld indicates the lock was not held by this locker instance.
	ErrLockNotHeld = errors.New("lock not held")
	// ErrLockValueMismatch indicates the lock value doesn't match or lock expired.
	ErrLockValueMismatch = errors.New("lock value mismatch or lock has expired")
	// ErrLockValueType indicates the stored lock value has an unexpected type.
	ErrLockValueType = errors.New("lock value type error")
)
