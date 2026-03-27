//go:build windows

package lock

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = `Global\wslwatch-instance`

// MutexLock implements single-instance lock using a Windows named mutex.
type MutexLock struct {
	handle windows.Handle
}

// NewLock creates a new platform-appropriate lock.
func NewLock() Lock {
	return &MutexLock{}
}

// Acquire attempts to acquire the named mutex.
func (l *MutexLock) Acquire() (bool, error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return false, fmt.Errorf("converting mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS {
			// Another instance holds the mutex
			_ = windows.CloseHandle(handle)
			return false, nil
		}
		return false, fmt.Errorf("creating mutex: %w", err)
	}

	// Try to acquire
	ret, _ := windows.WaitForSingleObject(handle, 0)
	if ret == uint32(windows.WAIT_OBJECT_0) || ret == uint32(windows.WAIT_ABANDONED) {
		l.handle = handle
		return true, nil
	}

	_ = windows.CloseHandle(handle)
	return false, nil
}

// Release releases the named mutex.
func (l *MutexLock) Release() error {
	if l.handle != 0 {
		_ = windows.ReleaseMutex(l.handle)
		_ = windows.CloseHandle(l.handle)
		l.handle = 0
	}
	return nil
}

// Ensure unsafe is used (required for windows package)
var _ = unsafe.Sizeof(0)
