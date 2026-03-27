//go:build windows

package service

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"syscall"
	"unsafe"

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
// serviceUser: Windows account to run the service as (e.g. ".\benja")
//
// Install is idempotent: if the service already exists it is stopped, the
// binary is updated, the config is refreshed, and the service is restarted.
func Install(copyBinary string, addToPath bool, serviceUser string) error {
	// 1. Determine install directory.
	installDir := filepath.Join(os.Getenv("PROGRAMDATA"), "wslwatch")

	// 2. Create install directory.
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("creating install dir %q: %w", installDir, err)
	}

	// 3. Connect to SCM early so we can stop a running service before
	//    overwriting the binary (Windows locks executables of running processes).
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to SCM: %w", err)
	}
	defer m.Disconnect()

	destBinary := filepath.Join(installDir, "wslwatch.exe")

	// 4. If the service already exists, stop it so the binary file is not locked.
	existing, _ := m.OpenService(ServiceName)
	if existing != nil {
		defer existing.Close()
		stopServiceWait(existing)
	}

	// 5. Copy binary (safe now that any running instance has been stopped).
	if err := copyFile(copyBinary, destBinary); err != nil {
		return fmt.Errorf("copying binary to %q: %w", destBinary, err)
	}

	// 6. Optionally add install dir to system PATH.
	if addToPath {
		if err := addToSystemPath(installDir); err != nil {
			log.Printf("warning: could not add %q to system PATH: %v", installDir, err)
		}
	}

	// 6b. Prompt for password if running as a specific user.
	var password string
	if serviceUser != "" {
		fmt.Printf("Service will run as %s\n", serviceUser)
		fmt.Print("Enter Windows password: ")
		pw, err := readPassword()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = pw
	}

	// 7. Create or update the service.
	var s *mgr.Service
	if existing != nil {
		// Service already registered — update its config in place.
		// ServiceType must be set explicitly (zero value is invalid when
		// changing ServiceStartName).
		cfg := mgr.Config{
			ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
			DisplayName:      DisplayName,
			StartType:        mgr.StartAutomatic,
			Description:      Description,
			BinaryPathName:   destBinary,
			ServiceStartName: serviceUser,
			Password:         password,
		}
		if err := existing.UpdateConfig(cfg); err != nil {
			return fmt.Errorf("updating service config: %w", err)
		}
		s = existing
	} else {
		// First-time installation.
		s, err = m.CreateService(ServiceName, destBinary, mgr.Config{
			DisplayName:      DisplayName,
			StartType:        mgr.StartAutomatic,
			Description:      Description,
			ServiceStartName: serviceUser,
			Password:         password,
		})
		if err != nil {
			return fmt.Errorf("creating service: %w", err)
		}
		defer s.Close()
	}

	// 8. (Re-)configure failure actions.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 0); err != nil {
		return fmt.Errorf("setting recovery actions: %w", err)
	}

	// 9. Grant "Log on as a service" right to the service user.
	if serviceUser != "" {
		if err := grantLogonAsService(serviceUser); err != nil {
			log.Printf("warning: could not grant SeServiceLogonRight to %s: %v", serviceUser, err)
		}
	}

	// 10. Start the service only if it is not already running.
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("querying service status: %w", err)
	}
	if status.State != svc.Running && status.State != svc.StartPending {
		if err := s.Start(); err != nil {
			return fmt.Errorf("starting service: %w", err)
		}
	}

	fmt.Println("wslwatch service installed and started")
	return nil
}

// stopServiceWait sends a stop control to s and waits up to 10 seconds for it
// to reach the Stopped state. Errors are ignored — the caller continues regardless.
func stopServiceWait(s *mgr.Service) {
	if _, err := s.Control(svc.Stop); err != nil {
		return // wasn't running, nothing to do
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil || status.State == svc.Stopped {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// readPassword reads a line from stdin with console echo disabled.
func readPassword() (string, error) {
	handle := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Fallback: read with echo (e.g., redirected stdin).
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	if err := windows.SetConsoleMode(handle, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return "", fmt.Errorf("disabling echo: %w", err)
	}
	defer windows.SetConsoleMode(handle, mode)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	fmt.Println() // newline after hidden input
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// LSA API types and procedures for granting user rights.
var (
	modAdvapi32              = syscall.NewLazyDLL("advapi32.dll")
	procLsaOpenPolicy        = modAdvapi32.NewProc("LsaOpenPolicy")
	procLsaAddAccountRights  = modAdvapi32.NewProc("LsaAddAccountRights")
	procLsaClose             = modAdvapi32.NewProc("LsaClose")
	procLsaNtStatusToWinError = modAdvapi32.NewProc("LsaNtStatusToWinError")
)

type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type lsaObjectAttributes struct {
	Length                   uint32
	RootDirectory            uintptr
	ObjectName               uintptr
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

// grantLogonAsService grants the SeServiceLogonRight to the given account.
func grantLogonAsService(accountName string) error {
	// Strip .\ prefix for SID lookup.
	lookupName := accountName
	if strings.HasPrefix(lookupName, `.\`) {
		lookupName = lookupName[2:]
	}

	// Lookup SID.
	sid, _, _, err := windows.LookupSID("", lookupName)
	if err != nil {
		return fmt.Errorf("looking up SID for %q: %w", lookupName, err)
	}

	// Open LSA policy.
	const policyCreateAccount = 0x00000010
	const policyLookupNames = 0x00000800
	var attrs lsaObjectAttributes
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var policyHandle uintptr
	ntStatus, _, _ := procLsaOpenPolicy.Call(
		0, // local system
		uintptr(unsafe.Pointer(&attrs)),
		policyCreateAccount|policyLookupNames,
		uintptr(unsafe.Pointer(&policyHandle)),
	)
	if ntStatus != 0 {
		winErr, _, _ := procLsaNtStatusToWinError.Call(ntStatus)
		return fmt.Errorf("LsaOpenPolicy: error code %d", winErr)
	}
	defer procLsaClose.Call(policyHandle)

	// Build the LSA_UNICODE_STRING for "SeServiceLogonRight".
	right, err := windows.UTF16FromString("SeServiceLogonRight")
	if err != nil {
		return err
	}
	lsaRight := lsaUnicodeString{
		Length:        uint16(len("SeServiceLogonRight") * 2),
		MaximumLength: uint16(len(right) * 2),
		Buffer:        &right[0],
	}

	// Grant the right.
	ntStatus, _, _ = procLsaAddAccountRights.Call(
		policyHandle,
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(&lsaRight)),
		1,
	)
	if ntStatus != 0 {
		winErr, _, _ := procLsaNtStatusToWinError.Call(ntStatus)
		return fmt.Errorf("LsaAddAccountRights: error code %d", winErr)
	}

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
// It always broadcasts WM_SETTINGCHANGE so that running processes pick up the
// current PATH, even if no registry write was needed.
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

	// Add dir to PATH if not already present.
	found := false
	for _, segment := range filepath.SplitList(currentPath) {
		if filepath.Clean(segment) == filepath.Clean(dir) {
			found = true
			break
		}
	}
	if !found {
		newPath := currentPath + string(os.PathListSeparator) + dir
		if err := key.SetExpandStringValue("Path", newPath); err != nil {
			return fmt.Errorf("writing PATH to registry: %w", err)
		}
	}

	// Broadcast WM_SETTINGCHANGE so running processes (Explorer, terminals)
	// pick up the new PATH without requiring a reboot/re-login.
	broadcastSettingChange()

	return nil
}

// broadcastSettingChange sends WM_SETTINGCHANGE to all top-level windows so
// they re-read environment variables from the registry.
func broadcastSettingChange() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(
		0xFFFF,          // HWND_BROADCAST
		uintptr(0x001A), // WM_SETTINGCHANGE
		0,
		uintptr(unsafe.Pointer(env)),
		0x0002, // SMTO_ABORTIFHUNG
		5000,   // timeout ms
		0,
	)
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
	stopServiceWait(s)

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
