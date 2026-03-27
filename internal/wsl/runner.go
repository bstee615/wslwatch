package wsl

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes wsl.exe commands with timeout support.
// It is defined as an interface to allow mocking in tests.
type Runner interface {
	// Run executes a wsl.exe command with the given arguments and timeout.
	Run(ctx context.Context, args ...string) (string, error)
}

// ExecRunner is the production implementation of Runner that calls wsl.exe.
type ExecRunner struct {
	// WSLPath is the path to wsl.exe. Defaults to "wsl.exe".
	WSLPath string
}

// NewExecRunner creates a new ExecRunner with default settings.
func NewExecRunner() *ExecRunner {
	return &ExecRunner{WSLPath: "wsl.exe"}
}

// Run executes wsl.exe with the given arguments and timeout context.
func (r *ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	wslPath := r.WSLPath
	if wslPath == "" {
		wslPath = "wsl.exe"
	}

	cmd := exec.CommandContext(ctx, wslPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("wsl.exe %s timed out: %w", strings.Join(args, " "), ctx.Err())
		}
		return "", fmt.Errorf("wsl.exe %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// RunWithTimeout is a convenience method that creates a context with timeout and calls Run.
func RunWithTimeout(r Runner, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.Run(ctx, args...)
}

// MockRunner is a test double for Runner.
type MockRunner struct {
	// Handler is called for each Run invocation. Return the desired output and error.
	Handler func(args []string) (string, error)
}

// Run calls the Handler function.
func (m *MockRunner) Run(_ context.Context, args ...string) (string, error) {
	if m.Handler != nil {
		return m.Handler(args)
	}
	return "", fmt.Errorf("MockRunner: no handler set")
}
