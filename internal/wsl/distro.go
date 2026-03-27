package wsl

import (
	"fmt"
	"strconv"
	"strings"
)

type DistroState string

const (
	StateRunning    DistroState = "Running"
	StateStopped    DistroState = "Stopped"
	StateInstalling DistroState = "Installing"
	StateUnknown    DistroState = "Unknown"
)

type DistroInfo struct {
	Name    string
	State   DistroState
	Default bool
	Version int // WSL version (1 or 2)
}

// ParseListVerbose parses the output of `wsl.exe --list --verbose`.
// Example output:
//
//	NAME                   STATE           VERSION
//
// * Ubuntu-22.04           Running         2
//
//	Ubuntu-20.04           Stopped         2
//
// cleanLine strips BOM characters, null bytes, and trailing whitespace/CRLF
// that wsl.exe sometimes includes in its output.
func cleanLine(line string) string {
	// UTF-8 BOM (\xef\xbb\xbf) and null bytes produced by wsl.exe on some systems.
	line = strings.ReplaceAll(line, "\xef\xbb\xbf", "")
	line = strings.ReplaceAll(line, "\x00", "")
	line = strings.TrimRight(line, "\r\n\t ")
	return line
}

func ParseListVerbose(output string) ([]DistroInfo, error) {
	lines := strings.Split(output, "\n")
	var distros []DistroInfo
	firstLine := true

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip header line
		if firstLine {
			firstLine = false
			continue
		}

		isDefault := false
		// Check for default marker (first char is '*')
		if strings.HasPrefix(line, "*") {
			isDefault = true
			// Replace '*' with ' ' to normalize for parsing
			line = " " + line[1:]
		}

		// Lines are in format:
		//   NAME                   STATE           VERSION
		// with leading spaces (2 spaces). After stripping leading spaces we split by whitespace.
		trimmed := strings.TrimLeft(line, " ")
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			return nil, fmt.Errorf("invalid line format (expected at least 3 fields): %q", line)
		}

		name := fields[0]
		stateStr := fields[1]
		versionStr := fields[2]

		var state DistroState
		switch stateStr {
		case "Running":
			state = StateRunning
		case "Stopped":
			state = StateStopped
		case "Installing":
			state = StateInstalling
		default:
			state = StateUnknown
		}

		version, err := strconv.Atoi(versionStr)
		if err != nil {
			return nil, fmt.Errorf("invalid version %q for distro %q: %w", versionStr, name, err)
		}

		distros = append(distros, DistroInfo{
			Name:    name,
			State:   state,
			Default: isDefault,
			Version: version,
		})
	}

	return distros, nil
}

// ParseListQuiet parses the output of `wsl.exe --list --quiet`.
// Returns list of distro names, one per line. Skips empty lines.
func ParseListQuiet(output string) ([]string, error) {
	lines := strings.Split(output, "\n")
	var names []string

	for _, line := range lines {
		line = strings.TrimSpace(cleanLine(line))

		if line == "" {
			continue
		}

		names = append(names, line)
	}

	return names, nil
}
