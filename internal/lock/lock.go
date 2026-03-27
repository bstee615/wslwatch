package lock

// Lock represents a single-instance lock.
type Lock interface {
	// Acquire attempts to acquire the lock.
	// Returns true if the lock was acquired, false if another instance holds it.
	Acquire() (bool, error)
	// Release releases the lock.
	Release() error
}
