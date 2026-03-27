//go:build !windows

package lock

import (
	"fmt"
	"os"
	"path/filepath"
)

const lockFile = "/tmp/wslwatch.lock"

// FileLock implements single-instance lock using a file lock on non-Windows platforms.
type FileLock struct {
	file *os.File
}

// NewLock creates a new platform-appropriate lock.
func NewLock() Lock {
	return &FileLock{}
}

// Acquire attempts to acquire the lock by creating a lock file.
func (l *FileLock) Acquire() (bool, error) {
	dir := filepath.Dir(lockFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("creating lock directory: %w", err)
	}

	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("creating lock file: %w", err)
	}
	_, _ = fmt.Fprintf(f, "%d", os.Getpid())
	l.file = f
	return true, nil
}

// Release releases the lock by removing the lock file.
func (l *FileLock) Release() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	return os.Remove(lockFile)
}
