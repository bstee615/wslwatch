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
	alreadyExists := errors.Is(err, windows.ERROR_ALREADY_EXISTS)
	if err != nil && !alreadyExists {
		return nil, fmt.Errorf("creating mutex: %w", err)
	}
	if handle == 0 {
		return nil, fmt.Errorf("another instance of wslwatch is already running")
	}

	// Non-blocking attempt to acquire ownership.
	ret, _ := windows.WaitForSingleObject(handle, 0)
	switch {
	case ret == uint32(windows.WAIT_ABANDONED):
		// Previous owner crashed; we now own it.
		return &Lock{handle: handle}, nil
	case ret == uint32(windows.WAIT_OBJECT_0) && !alreadyExists:
		// Fresh mutex, successfully acquired.
		return &Lock{handle: handle}, nil
	default:
		// Either WAIT_TIMEOUT (cross-process) or recursive lock (same-process).
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("another instance of wslwatch is already running")
	}
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
