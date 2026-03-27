//go:build integration

package integration

import (
	"flag"
	"os/exec"
	"strings"
	"testing"
)

var targetDistro = flag.String("distro", "", "target WSL distro for integration tests")

// getDistro returns the target distro for integration tests.
// If -distro flag is not set, it tries to auto-detect a suitable distro.
func getDistro(t *testing.T) string {
	t.Helper()
	if *targetDistro != "" {
		return *targetDistro
	}

	// Auto-detect: list distros and pick the first non-Docker one
	out, err := exec.Command("wsl.exe", "--list", "--quiet").Output()
	if err != nil {
		t.Skip("wsl.exe not available or no distros installed")
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "docker-desktop" || lower == "docker-desktop-data" {
			continue
		}
		return name
	}
	t.Skip("no suitable WSL distro found for integration tests")
	return ""
}

// isWSLAvailable checks if wsl.exe is available on the system.
func isWSLAvailable() bool {
	_, err := exec.LookPath("wsl.exe")
	return err == nil
}
