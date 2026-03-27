//go:build windows

package main

import "golang.org/x/sys/windows/svc"

// isWindowsService returns true if the process is running as a Windows Service.
func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return ok
}
