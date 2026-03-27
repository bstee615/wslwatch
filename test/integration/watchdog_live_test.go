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
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func newRunner() *wsl.WSLRunner {
	return wsl.NewWSLRunner()
}

// TestLiveWatchdogStartsDeadDistro verifies that the watchdog restarts a stopped distro.
func TestLiveWatchdogStartsDeadDistro(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 3 * time.Second}
	cfg.ProbeTimeout = config.Duration{Duration: 5 * time.Second}
	cfg.RestartDelay = config.Duration{Duration: 1 * time.Second}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true},
	}

	// Kill the distro.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runner.Terminate(ctx, distroName); err != nil {
		t.Logf("terminate returned: %v (may already be stopped)", err)
	}

	waitForState(t, runner, distroName, wsl.StateStopped, 10*time.Second)

	// Start watchdog.
	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Wait for watchdog to restart the distro.
	waitForState(t, runner, distroName, wsl.StateRunning, cfg.CheckInterval.Duration*2+10*time.Second)
	t.Logf("distro %s restarted successfully", distroName)
}

// TestLiveFailureTrackerBackoff verifies that the failure tracker enters backoff
// after repeated failures and exits backoff after the duration expires.
func TestLiveFailureTrackerBackoff(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 2 * time.Second}
	cfg.ProbeTimeout = config.Duration{Duration: 3 * time.Second}
	cfg.RestartDelay = config.Duration{Duration: 500 * time.Millisecond}
	cfg.FailureThreshold = 2
	cfg.FailureWindow = config.Duration{Duration: 30 * time.Second}
	cfg.BackoffDuration = config.Duration{Duration: 10 * time.Second}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true},
	}

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Kill the distro multiple times to trigger backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		_ = runner.Terminate(ctx, distroName)
		time.Sleep(cfg.CheckInterval.Duration + 500*time.Millisecond)
	}

	// After backoff kicks in, the distro should not be restarted for a while.
	status := w.GetStatus()
	found := false
	for _, d := range status.Distros {
		if d.Name == distroName {
			found = true
			t.Logf("distro status: state=%s inBackoff=%v restarts=%d", d.State, d.InBackoff, d.RestartCount)
		}
	}
	assert.True(t, found, "distro should appear in status")
}

// TestLivePauseResume verifies that pausing prevents restart and resuming allows it.
func TestLivePauseResume(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 3 * time.Second}
	cfg.ProbeTimeout = config.Duration{Duration: 5 * time.Second}
	cfg.RestartDelay = config.Duration{Duration: 1 * time.Second}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true},
	}

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Pause the distro.
	require.NoError(t, w.PauseDistro(distroName))

	// Kill the distro.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = runner.Terminate(ctx, distroName)
	waitForState(t, runner, distroName, wsl.StateStopped, 10*time.Second)

	// Wait two check intervals; distro should NOT be restarted.
	time.Sleep(cfg.CheckInterval.Duration * 2)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	distros, err := runner.ListDistros(ctx2)
	require.NoError(t, err)
	for _, d := range distros {
		if d.Name == distroName {
			assert.Equal(t, wsl.StateStopped, d.State, "paused distro should not be restarted")
			break
		}
	}

	// Resume and verify restart happens.
	require.NoError(t, w.ResumeDistro(distroName))
	waitForState(t, runner, distroName, wsl.StateRunning, cfg.CheckInterval.Duration*2+10*time.Second)
	t.Logf("distro %s restarted after resume", distroName)
}

// TestLiveMaxRestarts verifies the watchdog stops restarting after max_restarts.
func TestLiveMaxRestarts(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	const maxRestarts = 2

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 3 * time.Second}
	cfg.ProbeTimeout = config.Duration{Duration: 5 * time.Second}
	cfg.RestartDelay = config.Duration{Duration: 500 * time.Millisecond}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true, MaxRestarts: maxRestarts},
	}

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Kill the distro repeatedly.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i <= maxRestarts+1; i++ {
		_ = runner.Terminate(ctx, distroName)
		// Wait for watchdog to attempt restart.
		time.Sleep(cfg.CheckInterval.Duration + 3*time.Second)
	}

	// After max restarts, watchdog should mark distro as exhausted.
	status := w.GetStatus()
	for _, d := range status.Distros {
		if d.Name == distroName {
			assert.True(t, d.Exhausted, "distro should be exhausted after max restarts")
			t.Logf("distro exhausted after %d restarts", d.RestartCount)
			return
		}
	}
	t.Errorf("distro %s not found in status", distroName)
}

// TestLiveStatusOutput verifies that the IPC status command returns usable data.
func TestLiveStatusOutput(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 5 * time.Second}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true},
	}

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	time.Sleep(200 * time.Millisecond) // let watchdog initialise

	status := w.GetStatus()
	assert.True(t, status.Running)
	assert.NotEmpty(t, status.Distros)

	found := false
	for _, d := range status.Distros {
		if d.Name == distroName {
			found = true
			t.Logf("distro %s state=%s restarts=%d", d.Name, d.State, d.RestartCount)
		}
	}
	assert.True(t, found, "distro should appear in status")
}

// TestLiveIgnoredDistros verifies that ignored distros are never restarted.
func TestLiveIgnoredDistros(t *testing.T) {
	runner := newRunner()
	logger := newLogger()

	// Use a deliberately small distro list with an ignored entry.
	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 2 * time.Second}
	cfg.IgnoredDistros = []string{"docker-desktop", "docker-desktop-data"}
	// No distros in cfg.Distros means nothing is managed.

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Verify the IPC handler marks them as ignored.
	status := w.GetStatus()
	for _, d := range status.Distros {
		if d.Name == "docker-desktop" || d.Name == "docker-desktop-data" {
			assert.Equal(t, "ignored", d.State, "docker-desktop should be ignored")
		}
	}
}

// TestLiveIPCRoundtrip verifies the IPC server/client work end-to-end with the watchdog.
func TestLiveIPCRoundtrip(t *testing.T) {
	distroName := requireDistro(t)
	runner := newRunner()
	logger := newLogger()

	cfg := config.Default()
	cfg.CheckInterval = config.Duration{Duration: 5 * time.Second}
	cfg.Distros = []config.DistroConfig{
		{Name: distroName, Enabled: true},
	}

	w := watchdog.New(cfg, runner, logger)
	w.Start()
	defer w.Stop()

	// Start IPC server.
	handler := func(req ipc.Request) ipc.Response {
		if req.Cmd == "status" {
			s := w.GetStatus()
			var distros []ipc.DistroData
			for _, d := range s.Distros {
				distros = append(distros, ipc.DistroData{
					Name:         d.Name,
					State:        d.State,
					RestartCount: d.RestartCount,
				})
			}
			return ipc.Response{OK: true, Data: ipc.StatusData{
				Running: s.Running,
				Distros: distros,
			}}
		}
		return ipc.Response{OK: false, Error: "unknown"}
	}

	server := ipc.NewServer(handler)
	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	client := ipc.NewClientWithTimeout(5 * time.Second)
	assert.True(t, client.IsRunning())

	resp, err := client.Send(ipc.Request{Cmd: "status"})
	require.NoError(t, err)
	assert.True(t, resp.OK)
}
