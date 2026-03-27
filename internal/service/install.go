//go:build windows

package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "wslwatch"
	serviceDisplay = "WSL Watchdog"
	serviceDesc    = "Monitors WSL2 distros and restarts them when they die."
)

// InstallDir returns the install directory path.
func InstallDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "wslwatch")
}

// Install registers wslwatch as a Windows Service.
func Install(logger *slog.Logger) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Create install directory
	installDir := InstallDir()
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	// Copy binary to install dir
	destPath := filepath.Join(installDir, "wslwatch.exe")
	if exePath != destPath {
		input, err := os.ReadFile(exePath)
		if err != nil {
			return fmt.Errorf("reading executable: %w", err)
		}
		if err := os.WriteFile(destPath, input, 0o755); err != nil {
			return fmt.Errorf("copying executable: %w", err)
		}
	}

	// Connect to SCM
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager (are you running as admin?): %w", err)
	}
	defer m.Disconnect()

	// Check if already installed
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %q is already installed", serviceName)
	}

	// Create service
	s, err = m.CreateService(serviceName, destPath, mgr.Config{
		DisplayName:  serviceDisplay,
		Description:  serviceDesc,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	})
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}
	defer s.Close()

	// Configure failure actions: restart after 5s, 10s, 30s
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 0)
	if err != nil {
		logger.Warn("failed to set recovery actions", "error", err)
	}

	// Start the service
	if err := s.Start(); err != nil {
		logger.Warn("failed to start service", "error", err)
	}

	logger.Info("service installed and started",
		"name", serviceName,
		"path", destPath,
	)
	return nil
}

// Uninstall removes the wslwatch Windows Service.
func Uninstall(logger *slog.Logger, removeAll bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("opening service %q: %w", serviceName, err)
	}

	// Stop the service
	_, _ = s.Control(mgr.Stop)
	time.Sleep(2 * time.Second) // Give it time to stop

	// Delete the service
	if err := s.Delete(); err != nil {
		s.Close()
		return fmt.Errorf("deleting service: %w", err)
	}
	s.Close()

	logger.Info("service removed", "name", serviceName)

	if removeAll {
		installDir := InstallDir()
		if err := os.RemoveAll(installDir); err != nil {
			logger.Warn("failed to remove install directory", "dir", installDir, "error", err)
		} else {
			logger.Info("install directory removed", "dir", installDir)
		}
	}

	return nil
}
