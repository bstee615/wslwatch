//go:build !windows

package lock

import (
	"fmt"
	"os"
)

const lockFile = "/tmp/wslwatch.lock"

// Lock holds an acquired file-based instance lock.
type Lock struct {
	file *os.File
}

// Acquire creates an exclusive lock file to prevent duplicate instances.
// Uses O_EXCL so only one process can create it at a time.
func Acquire() (*Lock, error) {
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another instance of wslwatch is already running")
		}
		return nil, fmt.Errorf("creating lock file: %w", err)
	}
	_, _ = fmt.Fprintf(f, "%d", os.Getpid())
	return &Lock{file: f}, nil
}

// Release removes the lock file.
func (l *Lock) Release() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	return os.Remove(lockFile)
}
