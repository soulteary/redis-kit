package lock

// Locker provides distributed lock functionality
// Compatible with gocron.Locker interface and similar use cases
type Locker interface {
	// Lock acquires a distributed lock
	// Returns true if the lock was successfully acquired, false if the lock is already held
	Lock(key string) (bool, error)

	// Unlock releases a distributed lock
	// Returns an error if the lock cannot be released (e.g., lock value mismatch or lock expired)
	Unlock(key string) error
}
