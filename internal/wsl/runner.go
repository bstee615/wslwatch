package wsl

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// KeepAlive represents a running keep-alive process for a distro.
type KeepAlive interface {
	// Alive returns true if the keep-alive process is still running.
	Alive() bool
	// Stop terminates the keep-alive process and waits for it to exit.
	Stop()
}

// Runner defines the interface for interacting with WSL distros.
type Runner interface {
	// ListDistros returns all installed WSL distros with their states.
	ListDistros(ctx context.Context) ([]DistroInfo, error)

	// Probe sends a liveness probe to the distro (runs "echo ok").
	// Returns nil if distro responds with "ok", error otherwise.
	Probe(ctx context.Context, name string) error

	// Terminate shuts down a specific distro (wsl -t <name>).
	Terminate(ctx context.Context, name string) error

	// Start wakes a distro (wsl -d <name> -- true).
	Start(ctx context.Context, name string) error

	// Exec runs a command inside the distro, returns stdout.
	Exec(ctx context.Context, name string, args ...string) (string, error)

	// StartKeepAlive spawns a long-running no-op process inside the distro to
	// prevent it from shutting down due to having no running processes.
	StartKeepAlive(ctx context.Context, name string) (KeepAlive, error)
}

// WSLRunner is the real runner using wsl.exe.
type WSLRunner struct {
	WslPath string // path to wsl.exe, defaults to "wsl.exe"
}

// NewWSLRunner creates a new WSLRunner with the default wsl.exe path.
func NewWSLRunner() *WSLRunner {
	return &WSLRunner{WslPath: "wsl.exe"}
}

// runCommand runs a wsl.exe command and returns stdout. On non-zero exit it
// returns an error containing the stderr output.
func (r *WSLRunner) runCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.WslPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("wsl.exe %v: %w: %s", args, err, stderrStr)
		}
		return "", fmt.Errorf("wsl.exe %v: %w", args, err)
	}

	return decodeUTF16LE(stdout.Bytes()), nil
}

// decodeUTF16LE converts a UTF-16LE byte slice (with optional BOM) to a UTF-8 string.
// wsl.exe outputs UTF-16LE encoded text; this function normalises it to Go strings.
// If the input does not look like UTF-16LE (odd length or no null bytes) it is
// returned as-is so that non-Windows or mocked callers are unaffected.
func decodeUTF16LE(b []byte) string {
	if len(b) < 2 {
		return string(b)
	}

	// Heuristic: if there are no null bytes the output is probably already UTF-8.
	if !bytes.ContainsRune(b, 0) {
		return string(b)
	}

	// Strip BOM if present (FF FE).
	if b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}

	// Need an even number of bytes for UTF-16.
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}

	u16s := make([]uint16, len(b)/2)
	for i := range u16s {
		u16s[i] = binary.LittleEndian.Uint16(b[2*i:])
	}

	return string(utf16.Decode(u16s))
}

// ListDistros returns all installed WSL distros with their states.
func (r *WSLRunner) ListDistros(ctx context.Context) ([]DistroInfo, error) {
	out, err := r.runCommand(ctx, "--list", "--verbose")
	if err != nil {
		return nil, fmt.Errorf("listing distros: %w", err)
	}
	return ParseListVerbose(out)
}

// Probe sends a liveness probe to the distro by running "echo ok".
// Returns nil if distro responds with "ok", error otherwise.
func (r *WSLRunner) Probe(ctx context.Context, name string) error {
	out, err := r.runCommand(ctx, "-d", name, "-e", "echo", "ok")
	if err != nil {
		return fmt.Errorf("probe %q: %w", name, err)
	}
	out = strings.TrimRight(out, "\r\n")
	if out != "ok" {
		return fmt.Errorf("probe %q: unexpected response: %q", name, out)
	}
	return nil
}

// Terminate shuts down a specific distro.
func (r *WSLRunner) Terminate(ctx context.Context, name string) error {
	_, err := r.runCommand(ctx, "-t", name)
	if err != nil {
		return fmt.Errorf("terminate %q: %w", name, err)
	}
	return nil
}

// Start wakes a distro by running a no-op command inside it.
func (r *WSLRunner) Start(ctx context.Context, name string) error {
	_, err := r.runCommand(ctx, "-d", name, "--", "true")
	if err != nil {
		return fmt.Errorf("start %q: %w", name, err)
	}
	return nil
}

// Exec runs a command inside the distro and returns its stdout.
func (r *WSLRunner) Exec(ctx context.Context, name string, args ...string) (string, error) {
	wslArgs := append([]string{"-d", name, "-e"}, args...)
	out, err := r.runCommand(ctx, wslArgs...)
	if err != nil {
		return "", fmt.Errorf("exec %q %v: %w", name, args, err)
	}
	return out, nil
}

// keepAliveHandle is the real KeepAlive implementation backed by an OS process.
type keepAliveHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (h *keepAliveHandle) Alive() bool {
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}

func (h *keepAliveHandle) Stop() {
	h.cancel()
	<-h.done
}

// StartKeepAlive spawns "wsl -d <name> -- sleep infinity" as a background process.
func (r *WSLRunner) StartKeepAlive(_ context.Context, name string) (KeepAlive, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, r.WslPath, "-d", name, "--", "sleep", "infinity")
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start keep-alive for %q: %w", name, err)
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	return &keepAliveHandle{cancel: cancel, done: done}, nil
}

// MockRunner is a mock implementation of Runner for testing.
type MockRunner struct {
	Distros      []DistroInfo
	ProbeErr     map[string]error // distro name -> error to return
	TerminateErr map[string]error
	StartErr     map[string]error
	ExecResults    map[string]string // "name:cmd" -> result
	KeepAliveErr   map[string]error  // distro name -> error to return

	// Track calls
	ListCalls      int
	ProbeCalls     map[string]int
	TerminateCalls map[string]int
	StartCalls     map[string]int
	KeepAliveCalls map[string]int
}

// NewMockRunner creates a new MockRunner with initialized maps.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		ProbeErr:       make(map[string]error),
		TerminateErr:   make(map[string]error),
		StartErr:       make(map[string]error),
		ExecResults:    make(map[string]string),
		KeepAliveErr:   make(map[string]error),
		ProbeCalls:     make(map[string]int),
		TerminateCalls: make(map[string]int),
		StartCalls:     make(map[string]int),
		KeepAliveCalls: make(map[string]int),
	}
}

// ListDistros returns the mock distro list.
func (m *MockRunner) ListDistros(ctx context.Context) ([]DistroInfo, error) {
	m.ListCalls++
	return m.Distros, nil
}

// Probe records the call and returns the configured error (if any).
func (m *MockRunner) Probe(ctx context.Context, name string) error {
	m.ProbeCalls[name]++
	if err, ok := m.ProbeErr[name]; ok {
		return err
	}
	return nil
}

// Terminate records the call and returns the configured error (if any).
func (m *MockRunner) Terminate(ctx context.Context, name string) error {
	m.TerminateCalls[name]++
	if err, ok := m.TerminateErr[name]; ok {
		return err
	}
	return nil
}

// Start records the call and returns the configured error (if any).
func (m *MockRunner) Start(ctx context.Context, name string) error {
	m.StartCalls[name]++
	if err, ok := m.StartErr[name]; ok {
		return err
	}
	return nil
}

// Exec returns the pre-configured result for "name:cmd" key, or empty string.
func (m *MockRunner) Exec(ctx context.Context, name string, args ...string) (string, error) {
	key := name + ":" + strings.Join(args, " ")
	result, ok := m.ExecResults[key]
	if !ok {
		return "", nil
	}
	return result, nil
}

// MockKeepAlive is a mock KeepAlive for testing.
type MockKeepAlive struct {
	alive bool
}

func (h *MockKeepAlive) Alive() bool { return h.alive }
func (h *MockKeepAlive) Stop()       { h.alive = false }

// StartKeepAlive records the call and returns a mock keep-alive handle.
func (m *MockRunner) StartKeepAlive(_ context.Context, name string) (KeepAlive, error) {
	m.KeepAliveCalls[name]++
	if err, ok := m.KeepAliveErr[name]; ok {
		return nil, err
	}
	return &MockKeepAlive{alive: true}, nil
}
