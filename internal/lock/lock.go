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

// Acquire creates or opens the named mutex. If another instance already owns
// the mutex (ERROR_ALREADY_EXISTS returned by CreateMutex) an error is
// returned so the caller knows it is not the sole instance.
func Acquire() (*Lock, error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, fmt.Errorf("encoding mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, true, name)
	if err != nil {
		// CreateMutex returns the handle AND an error when the mutex already
		// exists; the error is ERROR_ALREADY_EXISTS.
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			// Close the handle we just received – we don't own the mutex.
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			return nil, fmt.Errorf("another instance of wslwatch is already running")
		}
		return nil, fmt.Errorf("creating mutex: %w", err)
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
