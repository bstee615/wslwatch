//go:build !windows

package service

import (
	"log/slog"

	"github.com/bstee615/wslwatch/internal/config"
)

// Install is a no-op on non-Windows platforms.
func Install(_ *slog.Logger) error {
	return nil
}

// Uninstall is a no-op on non-Windows platforms.
func Uninstall(_ *slog.Logger, _ bool) error {
	return nil
}

// InstallDir returns the install directory for non-Windows.
func InstallDir() string {
	return "/opt/wslwatch"
}

// RunService is a no-op on non-Windows.
func RunService(_ *config.Config, _ string, _ *slog.Logger) error {
	return nil
}

// IsWindowsService always returns false on non-Windows.
func IsWindowsService() bool {
	return false
}
