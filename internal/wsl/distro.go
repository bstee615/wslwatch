package wsl

import (
	"context"
	"strings"
	"time"
)

// DistroState represents the running state of a WSL distro.
type DistroState string

const (
	StateRunning    DistroState = "Running"
	StateStopped    DistroState = "Stopped"
	StateInstalling DistroState = "Installing"
	StateUnknown    DistroState = "Unknown"
)

// DistroInfo holds parsed information about a WSL distro.
type DistroInfo struct {
	Name      string
	State     DistroState
	IsDefault bool
	Version   int // WSL version (1 or 2)
}

// ListDistros enumerates installed WSL distros by parsing `wsl --list --verbose`.
func ListDistros(r Runner, timeout time.Duration) ([]DistroInfo, error) {
	output, err := RunWithTimeout(r, timeout, "--list", "--verbose")
	if err != nil {
		return nil, err
	}
	return ParseListVerbose(output), nil
}

// ParseListVerbose parses the output of `wsl --list --verbose`.
// The output format is:
//
//	  NAME                   STATE           VERSION
//	* Ubuntu-22.04           Running         2
//	  docker-desktop         Stopped         2
func ParseListVerbose(output string) []DistroInfo {
	var distros []DistroInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// Clean the line of BOM and other control characters
		line = cleanLine(line)
		if line == "" {
			continue
		}

		// Skip header line
		lower := strings.ToLower(line)
		if strings.Contains(lower, "name") && strings.Contains(lower, "state") && strings.Contains(lower, "version") {
			continue
		}

		info := parseLine(line)
		if info != nil {
			distros = append(distros, *info)
		}
	}
	return distros
}

func cleanLine(line string) string {
	// Remove BOM and null bytes that wsl.exe sometimes outputs
	line = strings.ReplaceAll(line, "\xef\xbb\xbf", "")
	line = strings.ReplaceAll(line, "\x00", "")
	line = strings.TrimRight(line, "\r\n\t ")
	return line
}

func parseLine(line string) *DistroInfo {
	info := &DistroInfo{}

	// Check for default marker
	if strings.HasPrefix(line, "*") {
		info.IsDefault = true
		line = strings.TrimPrefix(line, "*")
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return nil
	}

	// Split into fields
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil
	}

	info.Name = fields[0]

	// Parse state
	switch strings.ToLower(fields[1]) {
	case "running":
		info.State = StateRunning
	case "stopped":
		info.State = StateStopped
	case "installing":
		info.State = StateInstalling
	default:
		info.State = StateUnknown
	}

	// Parse version if present
	if len(fields) >= 3 {
		switch fields[2] {
		case "1":
			info.Version = 1
		case "2":
			info.Version = 2
		}
	}

	return info
}

// ListDistroNames enumerates distro names using `wsl --list --quiet`.
func ListDistroNames(r Runner, timeout time.Duration) ([]string, error) {
	output, err := RunWithTimeout(r, timeout, "--list", "--quiet")
	if err != nil {
		return nil, err
	}
	return parseListQuiet(output), nil
}

func parseListQuiet(output string) []string {
	var names []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = cleanLine(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names
}

// ProbeDistro sends a liveness probe to a distro by running `echo ok`.
func ProbeDistro(r Runner, name string, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := r.Run(ctx, "-d", name, "-e", "echo", "ok")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "ok", nil
}

// TerminateDistro terminates a single distro with `wsl -t <name>`.
func TerminateDistro(r Runner, name string, timeout time.Duration) error {
	_, err := RunWithTimeout(r, timeout, "-t", name)
	return err
}

// StartDistro wakes a distro with `wsl -d <name> -- true`.
func StartDistro(r Runner, name string, timeout time.Duration) error {
	_, err := RunWithTimeout(r, timeout, "-d", name, "--", "true")
	return err
}

// RunInDistro runs a command inside a distro.
func RunInDistro(r Runner, name string, timeout time.Duration, command string) (string, error) {
	return RunWithTimeout(r, timeout, "-d", name, "-e", "sh", "-c", command)
}

// IsInstallingDistro checks if a distro name represents one that's installing.
func IsInstallingDistro(info DistroInfo) bool {
	return info.State == StateInstalling
}

// IsDockerDistro checks if a distro name is a known Docker integration distro.
func IsDockerDistro(name string) bool {
	lower := strings.ToLower(name)
	return lower == "docker-desktop" || lower == "docker-desktop-data"
}
