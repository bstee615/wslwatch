package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/wsl"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testConfig() *config.Config {
	return &config.Config{
		LogLevel:         "debug",
		CheckInterval:    100 * time.Millisecond,
		ProbeTimeout:     5 * time.Second,
		RestartDelay:     10 * time.Millisecond,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 5,
		BackoffDuration:  0,
		Distros: []config.DistroConfig{
			{Name: "TestDistro", Enabled: true},
		},
		IgnoredDistros: []string{"docker-desktop"},
	}
}

func TestWatchdog_StartsAndStops(t *testing.T) {
	cfg := testConfig()
	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n* TestDistro Running 2", nil
			}
			if len(args) >= 4 && args[0] == "-d" && args[2] == "-e" {
				return "ok", nil
			}
			return "", nil
		},
	}

	w := New(cfg, mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := w.Run(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWatchdog_DetectsUnhealthyAndRestarts(t *testing.T) {
	cfg := testConfig()
	var restartCount atomic.Int32

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n  TestDistro Stopped 2", nil
			}
			// Terminate
			if len(args) >= 2 && args[0] == "-t" {
				return "", nil
			}
			// Start
			if len(args) >= 3 && args[0] == "-d" && args[2] == "--" {
				return "", nil
			}
			return "", nil
		},
	}

	w := New(cfg, mock, testLogger())
	w.OnRestart = func(distro string) {
		restartCount.Add(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)
	assert.Greater(t, restartCount.Load(), int32(0))
}

func TestWatchdog_MaxRestarts(t *testing.T) {
	cfg := testConfig()
	cfg.Distros[0].MaxRestarts = 2
	var restartCount atomic.Int32

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n  TestDistro Stopped 2", nil
			}
			return "", nil
		},
	}

	w := New(cfg, mock, testLogger())
	w.OnRestart = func(distro string) {
		restartCount.Add(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = w.Run(ctx)
	// Should stop at max_restarts
	assert.LessOrEqual(t, restartCount.Load(), int32(2))
}

func TestWatchdog_PauseResume(t *testing.T) {
	cfg := testConfig()
	var restartCount atomic.Int32

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n  TestDistro Stopped 2", nil
			}
			return "", nil
		},
	}

	w := New(cfg, mock, testLogger())
	w.OnRestart = func(distro string) {
		restartCount.Add(1)
	}

	// Pause distro
	err := w.PauseDistro("TestDistro")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)
	// Should not restart while paused
	assert.Equal(t, int32(0), restartCount.Load())
}

func TestWatchdog_PauseDistro_NotFound(t *testing.T) {
	cfg := testConfig()
	mock := &wsl.MockRunner{Handler: func(args []string) (string, error) { return "", nil }}
	w := New(cfg, mock, testLogger())

	err := w.PauseDistro("NonExistent")
	assert.ErrorContains(t, err, "not found")
}

func TestWatchdog_ResumeDistro_NotFound(t *testing.T) {
	cfg := testConfig()
	mock := &wsl.MockRunner{Handler: func(args []string) (string, error) { return "", nil }}
	w := New(cfg, mock, testLogger())

	err := w.ResumeDistro("NonExistent")
	assert.ErrorContains(t, err, "not found")
}

func TestWatchdog_Status(t *testing.T) {
	cfg := testConfig()
	mock := &wsl.MockRunner{Handler: func(args []string) (string, error) { return "", nil }}
	w := New(cfg, mock, testLogger())

	ws := w.Status()
	assert.True(t, ws.Running)
	assert.Len(t, ws.Distros, 1)
	assert.Equal(t, "TestDistro", ws.Distros[0].Name)
}

func TestWatchdog_PauseAll(t *testing.T) {
	cfg := testConfig()
	var tickCount atomic.Int32

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			tickCount.Add(1)
			return "", fmt.Errorf("should not be called when paused")
		},
	}

	w := New(cfg, mock, testLogger())
	w.PauseAll()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)
	// No WSL calls should have been made
	assert.Equal(t, int32(0), tickCount.Load())
}

func TestWatchdog_ReloadConfig(t *testing.T) {
	cfg := testConfig()
	mock := &wsl.MockRunner{Handler: func(args []string) (string, error) { return "", nil }}
	w := New(cfg, mock, testLogger())

	// Initially one distro
	assert.Len(t, w.Status().Distros, 1)

	// Reload with additional distro
	newCfg := testConfig()
	newCfg.Distros = append(newCfg.Distros, config.DistroConfig{
		Name:    "NewDistro",
		Enabled: true,
	})
	w.ReloadConfig(newCfg)

	assert.Len(t, w.Status().Distros, 2)
}

func TestWatchdog_HealthyDistroNotRestarted(t *testing.T) {
	cfg := testConfig()
	var restartCount atomic.Int32

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) >= 2 && args[0] == "--list" && args[1] == "--verbose" {
				return "  NAME     STATE     VERSION\n* TestDistro Running 2", nil
			}
			if len(args) >= 4 && args[0] == "-d" && args[2] == "-e" {
				return "ok", nil
			}
			return "", nil
		},
	}

	w := New(cfg, mock, testLogger())
	w.OnRestart = func(distro string) {
		restartCount.Add(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)
	assert.Equal(t, int32(0), restartCount.Load())
}
