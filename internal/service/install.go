//go:build windows

package service

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	ServiceName = "wslwatch"
	DisplayName = "WSL Watchdog"
	Description = "Monitors WSL2 distros and restarts them when they die"
	InstallDir  = `wslwatch` // relative to ProgramData
)

// Install installs the wslwatch service.
// copyBinary: path to the binary to install (usually os.Executable())
// addToPath: whether to add the install dir to the system PATH
func Install(copyBinary string, addToPath bool) error {
	// 1. Determine install directory.
	installDir := filepath.Join(os.Getenv("PROGRAMDATA"), "wslwatch")

	// 2. Create install directory.
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("creating install dir %q: %w", installDir, err)
	}

	// 3. Copy binary to install dir as wslwatch.exe.
	destBinary := filepath.Join(installDir, "wslwatch.exe")
	if err := copyFile(copyBinary, destBinary); err != nil {
		return fmt.Errorf("copying binary to %q: %w", destBinary, err)
	}

	// 4. Optionally add install dir to system PATH.
	if addToPath {
		if err := addToSystemPath(installDir); err != nil {
			log.Printf("warning: could not add %q to system PATH: %v", installDir, err)
		}
	}

	// 5. Connect to SCM.
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to SCM: %w", err)
	}
	defer m.Disconnect()

	// 6. Create service.
	s, err := m.CreateService(ServiceName, destBinary, mgr.Config{
		DisplayName: DisplayName,
		StartType:   mgr.StartAutomatic,
		Description: Description,
	})
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}
	defer s.Close()

	// 7. Configure failure actions: restart after 5s, 10s, 30s.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 0); err != nil {
		return fmt.Errorf("setting recovery actions: %w", err)
	}

	// 8. Start the service.
	if err := s.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	// 9. Confirm.
	fmt.Println("wslwatch service installed and started")
	return nil
}

// copyFile copies src to dst, creating or overwriting dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// addToSystemPath appends dir to the system PATH in the registry, if not already present.
func addToSystemPath(dir string) error {
	const regPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, regPath,
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening registry key: %w", err)
	}
	defer key.Close()

	currentPath, _, err := key.GetStringValue("Path")
	if err != nil {
		return fmt.Errorf("reading PATH from registry: %w", err)
	}

	// Check if dir is already in PATH.
	for _, segment := range filepath.SplitList(currentPath) {
		if filepath.Clean(segment) == filepath.Clean(dir) {
			return nil
		}
	}

	newPath := currentPath + string(os.PathListSeparator) + dir
	if err := key.SetExpandStringValue("Path", newPath); err != nil {
		return fmt.Errorf("writing PATH to registry: %w", err)
	}
	return nil
}

// Uninstall removes the wslwatch service.
// removeAll: if true, also removes the install directory.
func Uninstall(removeAll bool) error {
	// 1. Connect to SCM.
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to SCM: %w", err)
	}
	defer m.Disconnect()

	// 2. Open service.
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("opening service %q: %w", ServiceName, err)
	}
	defer s.Close()

	// 3. Stop the service.
	if _, err := s.Control(svc.Stop); err != nil {
		// If it wasn't running, continue.
		log.Printf("warning: stopping service: %v", err)
	} else {
		// Wait up to 30 seconds for it to stop.
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			status, err := s.Query()
			if err != nil {
				break
			}
			if status.State == svc.Stopped {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 4. Delete service.
	if err := s.Delete(); err != nil {
		return fmt.Errorf("deleting service: %w", err)
	}

	// 5. Optionally remove install dir.
	if removeAll {
		installDir := filepath.Join(os.Getenv("PROGRAMDATA"), "wslwatch")
		if err := os.RemoveAll(installDir); err != nil {
			return fmt.Errorf("removing install dir %q: %w", installDir, err)
		}
	}

	return nil
}

// IsElevated returns true if the current process has admin privileges.
func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()

	return token.IsElevated()
}

// RelaunchElevated re-launches the current executable with the same args using
// ShellExecuteEx with "runas" verb.
func RelaunchElevated(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}

	// Build parameter string.
	params := ""
	for i, a := range args {
		if i > 0 {
			params += " "
		}
		params += a
	}

	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return err
		}
	}

	return windows.ShellExecute(0, verb, exePtr, paramsPtr, nil, windows.SW_SHOWNORMAL)
}
