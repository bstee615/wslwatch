//go:build windows

package lock

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

const mutexName = `Global\wslwatch-instance`

// Lock holds an acquired named Windows mutex handle.
type Lock struct {
	handle windows.Handle
}

// Acquire creates or opens the named mutex and acquires ownership.
// Returns an error if another instance already holds the mutex.
// Using WaitForSingleObject with a zero timeout handles the WAIT_ABANDONED
// case (previous owner crashed without releasing) gracefully.
func Acquire() (*Lock, error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, fmt.Errorf("encoding mutex name: %w", err)
	}

	// bInitialOwner=false: we acquire via WaitForSingleObject below, which
	// also handles the WAIT_ABANDONED case if a prior owner crashed.
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, fmt.Errorf("creating mutex: %w", err)
	}
	if handle == 0 {
		return nil, fmt.Errorf("another instance of wslwatch is already running")
	}

	// Non-blocking attempt to acquire ownership.
	ret, _ := windows.WaitForSingleObject(handle, 0)
	if ret != uint32(windows.WAIT_OBJECT_0) && ret != uint32(windows.WAIT_ABANDONED) {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("another instance of wslwatch is already running")
	}

	return &Lock{handle: handle}, nil
}

// Release releases ownership of the mutex and closes the handle.
func (l *Lock) Release() error {
	if err := windows.ReleaseMutex(l.handle); err != nil {
		return fmt.Errorf("releasing mutex: %w", err)
	}
	if err := windows.CloseHandle(l.handle); err != nil {
		return fmt.Errorf("closing mutex handle: %w", err)
	}
	return nil
}
