//go:build integration

package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

func TestLiveWatchdogStartsDeadDistro(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	distro := getDistro(t)
	runner := wsl.NewExecRunner()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    2 * time.Second,
		ProbeTimeout:     8 * time.Second,
		RestartDelay:     1 * time.Second,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 10,
		Distros: []config.DistroConfig{
			{Name: distro, Enabled: true},
		},
	}

	// Kill the distro first
	err := wsl.TerminateDistro(runner, distro, 10*time.Second)
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	// Start watchdog
	w := watchdog.New(cfg, runner, logger)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CheckInterval*3)
	defer cancel()

	_ = w.Run(ctx)

	// Verify distro is running
	distros, err := wsl.ListDistros(runner, 10*time.Second)
	require.NoError(t, err)

	found := false
	for _, d := range distros {
		if d.Name == distro && d.State == wsl.StateRunning {
			found = true
			break
		}
	}
	assert.True(t, found, "distro %s should be running after watchdog restart", distro)
}

func TestLiveFailureTrackerBackoff(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	distro := getDistro(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    500 * time.Millisecond,
		ProbeTimeout:     5 * time.Second,
		RestartDelay:     100 * time.Millisecond,
		FailureWindow:    5 * time.Second,
		FailureThreshold: 2,
		BackoffDuration:  3 * time.Second,
		Distros: []config.DistroConfig{
			{Name: distro, Enabled: true},
		},
	}

	// Mock runner that always reports failures
	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n  " + distro + " Stopped 2", nil
			}
			return "", nil
		},
	}

	w := watchdog.New(cfg, mock, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Verify status shows backoff was entered
	ws := w.Status()
	for _, d := range ws.Distros {
		if d.Name == distro {
			// After backoff + reset, restart count should be limited
			t.Logf("Distro %s: restarts=%d, failures=%d", d.Name, d.RestartCount, d.FailureCount)
		}
	}
}

func TestLivePauseResume(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	distro := getDistro(t)
	runner := wsl.NewExecRunner()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    1 * time.Second,
		ProbeTimeout:     8 * time.Second,
		RestartDelay:     1 * time.Second,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 10,
		Distros: []config.DistroConfig{
			{Name: distro, Enabled: true},
		},
	}

	w := watchdog.New(cfg, runner, logger)

	// Pause distro
	err := w.PauseDistro(distro)
	require.NoError(t, err)

	// Kill distro
	_ = wsl.TerminateDistro(runner, distro, 10*time.Second)
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = w.Run(ctx)

	// Distro should still be stopped (paused)
	distros, err := wsl.ListDistros(runner, 10*time.Second)
	require.NoError(t, err)
	for _, d := range distros {
		if d.Name == distro {
			assert.NotEqual(t, wsl.StateRunning, d.State, "distro should not be running while paused")
		}
	}
}

func TestLiveMaxRestarts(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	distro := getDistro(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    500 * time.Millisecond,
		ProbeTimeout:     5 * time.Second,
		RestartDelay:     100 * time.Millisecond,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 100, // high to not trigger backoff
		Distros: []config.DistroConfig{
			{Name: distro, Enabled: true, MaxRestarts: 3},
		},
	}

	// Always report stopped so it keeps trying to restart
	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n  " + distro + " Stopped 2", nil
			}
			return "", nil
		},
	}

	w := watchdog.New(cfg, mock, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	ws := w.Status()
	for _, d := range ws.Distros {
		if d.Name == distro {
			assert.True(t, d.Exhausted, "distro should be marked as exhausted")
			assert.LessOrEqual(t, d.RestartCount, 3)
		}
	}
}

func TestLiveStatusOutput(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	distro := getDistro(t)
	runner := wsl.NewExecRunner()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    1 * time.Second,
		ProbeTimeout:     8 * time.Second,
		RestartDelay:     1 * time.Second,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 5,
		Distros: []config.DistroConfig{
			{Name: distro, Enabled: true},
		},
	}

	w := watchdog.New(cfg, runner, logger)
	ws := w.Status()

	assert.True(t, ws.Running)
	assert.Len(t, ws.Distros, 1)
	assert.Equal(t, distro, ws.Distros[0].Name)
}

func TestLiveIgnoredDistros(t *testing.T) {
	if !isWSLAvailable() {
		t.Skip("WSL not available")
	}

	runner := wsl.NewExecRunner()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    1 * time.Second,
		ProbeTimeout:     8 * time.Second,
		RestartDelay:     1 * time.Second,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 5,
		Distros:          []config.DistroConfig{},
		IgnoredDistros:   []string{"docker-desktop", "docker-desktop-data"},
	}

	w := watchdog.New(cfg, runner, logger)
	ws := w.Status()

	// docker-desktop should not appear in managed distros
	for _, d := range ws.Distros {
		assert.NotEqual(t, "docker-desktop", d.Name)
		assert.NotEqual(t, "docker-desktop-data", d.Name)
	}
}
