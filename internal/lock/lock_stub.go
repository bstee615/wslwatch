//go:build !windows

package lock

// Lock is a no-op placeholder on non-Windows platforms.
type Lock struct{}

// Acquire always succeeds on non-Windows platforms.
func Acquire() (*Lock, error) { return &Lock{}, nil }

// Release is a no-op on non-Windows platforms.
func (l *Lock) Release() error { return nil }
