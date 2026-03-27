package wsl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureVMIdleTimeout ensures vmIdleTimeout=0 is set under [wsl2] in
// the user's .wslconfig file. This prevents the WSL 2 lightweight VM from
// auto-shutting down when idle.
func EnsureVMIdleTimeout() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir: %w", err)
	}
	return ensureVMIdleTimeoutInFile(filepath.Join(home, ".wslconfig"))
}

// ensureVMIdleTimeoutInFile is the testable core that operates on a specific path.
func ensureVMIdleTimeoutInFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// Normalise line endings for processing.
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimRight(text, "\n")

	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}

	inWsl2 := false
	found := false
	wsl2End := -1 // index of first line AFTER the [wsl2] keys

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if inWsl2 && !found {
				wsl2End = i
			}
			inWsl2 = strings.EqualFold(trimmed, "[wsl2]")
			continue
		}
		if inWsl2 {
			key := strings.SplitN(trimmed, "=", 2)[0]
			if strings.EqualFold(strings.TrimSpace(key), "vmidletimeout") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[1]) == "0" {
					return nil // already correct
				}
				lines[i] = "vmIdleTimeout=0"
				found = true
			}
		}
	}

	// If we were in [wsl2] at end of file and didn't find the key.
	if inWsl2 && !found {
		wsl2End = len(lines)
	}

	if !found {
		if wsl2End >= 0 {
			// Insert at end of [wsl2] section.
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:wsl2End]...)
			newLines = append(newLines, "vmIdleTimeout=0")
			newLines = append(newLines, lines[wsl2End:]...)
			lines = newLines
		} else {
			// No [wsl2] section exists; append one.
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "[wsl2]", "vmIdleTimeout=0")
		}
	}

	output := strings.Join(lines, "\r\n") + "\r\n"
	return os.WriteFile(path, []byte(output), 0644)
}
