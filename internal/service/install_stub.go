//go:build !windows

package service

import "errors"

const (
	ServiceName = "wslwatch"
	DisplayName = "WSL Watchdog"
	Description = "Monitors WSL2 distros and restarts them when they die"
	InstallDir  = "wslwatch"
)

func Install(copyBinary string, addToPath bool) error {
	return errors.New("service installation is only supported on Windows")
}

func Uninstall(removeAll bool) error {
	return errors.New("service uninstallation is only supported on Windows")
}

func IsElevated() bool { return true }

func RelaunchElevated(args ...string) error {
	return errors.New("elevation is only supported on Windows")
}
