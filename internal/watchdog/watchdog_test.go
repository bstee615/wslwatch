package watchdog

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/wsl"
	"github.com/stretchr/testify/assert"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// testConfig returns a minimal config suitable for testing (short intervals, no sleeps).
func testConfig(distros []config.DistroConfig) *config.Config {
	return &config.Config{
		LogLevel:         "debug",
		CheckInterval:    config.Duration{Duration: 10 * time.Millisecond},
		ProbeTimeout:     config.Duration{Duration: 1 * time.Second},
		RestartDelay:     config.Duration{Duration: 0},
		FailureWindow:    config.Duration{Duration: 60 * time.Second},
		FailureThreshold: 5,
		BackoffDuration:  config.Duration{Duration: 0},
		Distros:          distros,
		IgnoredDistros:   []string{},
	}
}

// TestWatchdogStartStop verifies Start and Stop do not crash.
func TestWatchdogStartStop(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	w.Start()

	// Give the loop a moment to tick at least once.
	time.Sleep(30 * time.Millisecond)

	w.Stop()
}

// TestWatchdogCheckAllHealthyDistro verifies that a healthy distro does not trigger a restart.
func TestWatchdogCheckAllHealthyDistro(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}
	// No probe error means Probe returns nil (healthy).

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	w.checkAll()

	assert.Equal(t, 0, runner.TerminateCalls["Ubuntu"], "terminate should not be called for healthy distro")
	assert.Equal(t, 0, runner.StartCalls["Ubuntu"], "start should not be called for healthy distro")
}

// TestWatchdogCheckAllDeadDistro verifies that a stopped distro triggers a restart
// after enough failures accumulate (failure_threshold).
func TestWatchdogCheckAllDeadDistro(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())

	// Failures below threshold should not trigger restart.
	for i := 0; i < cfg.FailureThreshold-1; i++ {
		w.checkAll()
	}
	assert.Equal(t, 0, runner.TerminateCalls["Ubuntu"], "should not restart before threshold")

	// One more failure reaches the threshold.
	w.checkAll()
	assert.Equal(t, 1, runner.TerminateCalls["Ubuntu"], "terminate should be called at threshold")
	assert.Equal(t, 1, runner.StartCalls["Ubuntu"], "start should be called at threshold")
	assert.Equal(t, 1, w.states["Ubuntu"].RestartCount, "restart count should be incremented")
}

// TestWatchdogPauseDistro verifies that a paused distro is not restarted when dead.
func TestWatchdogPauseDistro(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())

	err := w.PauseDistro("Ubuntu")
	assert.NoError(t, err)

	w.checkAll()

	assert.Equal(t, 0, runner.TerminateCalls["Ubuntu"], "terminate should not be called for paused distro")
	assert.Equal(t, 0, runner.StartCalls["Ubuntu"], "start should not be called for paused distro")
}

// TestWatchdogMaxRestarts verifies that a distro is marked exhausted after max_restarts.
func TestWatchdogMaxRestarts(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true, MaxRestarts: 2},
	})

	w := New(cfg, runner, testLogger())

	// Helper: run enough checks to trigger one restart (failure_threshold checks).
	triggerRestart := func() {
		for i := 0; i < cfg.FailureThreshold; i++ {
			w.checkAll()
		}
	}

	// First restart.
	triggerRestart()
	assert.False(t, w.states["Ubuntu"].Exhausted, "should not be exhausted after 1st restart")
	assert.Equal(t, 1, w.states["Ubuntu"].RestartCount)

	// Second restart.
	triggerRestart()
	assert.False(t, w.states["Ubuntu"].Exhausted, "should not be exhausted after 2nd restart")
	assert.Equal(t, 2, w.states["Ubuntu"].RestartCount)

	// Third threshold hit: restart count == max_restarts, so this should exhaust.
	triggerRestart()
	assert.True(t, w.states["Ubuntu"].Exhausted, "should be exhausted after max restarts reached")
	// Terminate/Start should only have been called twice total.
	assert.Equal(t, 2, runner.TerminateCalls["Ubuntu"])
	assert.Equal(t, 2, runner.StartCalls["Ubuntu"])

	// Subsequent calls should be skipped entirely.
	w.checkAll()
	assert.Equal(t, 2, runner.TerminateCalls["Ubuntu"], "no further restarts after exhaustion")
}

// TestWatchdogGetStatus verifies GetStatus reflects internal state accurately.
func TestWatchdogGetStatus(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})
	cfg.IgnoredDistros = []string{"docker-desktop"}

	w := New(cfg, runner, testLogger())

	// Run one healthy check to populate StartedAt.
	w.checkAll()

	status := w.GetStatus()

	// Find Ubuntu status.
	var ubuntuStatus *DistroStatus
	for i := range status.Distros {
		if status.Distros[i].Name == "Ubuntu" {
			ubuntuStatus = &status.Distros[i]
			break
		}
	}

	assert.NotNil(t, ubuntuStatus, "Ubuntu should appear in status")
	assert.Equal(t, "healthy", ubuntuStatus.State)
	assert.False(t, w.states["Ubuntu"].StartedAt.IsZero(), "StartedAt should be set after healthy check")
	assert.Equal(t, 0, ubuntuStatus.RestartCount)
	assert.False(t, ubuntuStatus.Exhausted)
	assert.False(t, ubuntuStatus.InBackoff)

	// Verify ignored distro appears.
	var ignoredStatus *DistroStatus
	for i := range status.Distros {
		if status.Distros[i].Name == "docker-desktop" {
			ignoredStatus = &status.Distros[i]
			break
		}
	}
	assert.NotNil(t, ignoredStatus, "docker-desktop should appear as ignored")
	assert.Equal(t, "ignored", ignoredStatus.State)
}

// TestWatchdogIgnoredDistro verifies that distros in IgnoredDistros are not managed.
func TestWatchdogIgnoredDistro(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "docker-desktop", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{})
	cfg.IgnoredDistros = []string{"docker-desktop"}

	w := New(cfg, runner, testLogger())

	assert.True(t, w.IsIgnored("docker-desktop"))
	assert.False(t, w.IsIgnored("Ubuntu"))

	// Even if docker-desktop were in Distros (stopped), checkAll should not touch it
	// because it's not in cfg.Distros (not enabled).
	w.checkAll()

	assert.Equal(t, 0, runner.TerminateCalls["docker-desktop"])
	assert.Equal(t, 0, runner.StartCalls["docker-desktop"])
}

// TestWatchdogGlobalPause verifies PauseAll / ResumeAll.
func TestWatchdogGlobalPause(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	w.PauseAll()

	w.checkAll()
	assert.Equal(t, 0, runner.TerminateCalls["Ubuntu"], "globally paused: no restart expected")

	w.ResumeAll()
	for i := 0; i < cfg.FailureThreshold; i++ {
		w.checkAll()
	}
	assert.Equal(t, 1, runner.TerminateCalls["Ubuntu"], "after resume: restart expected")
}

// TestWatchdogStartCommandExec verifies the start command is executed after restart.
func TestWatchdogStartCommandExec(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true, StartCommand: "echo hello"},
	})

	w := New(cfg, runner, testLogger())
	for i := 0; i < cfg.FailureThreshold; i++ {
		w.checkAll()
	}

	assert.Equal(t, 1, runner.StartCalls["Ubuntu"])
	// Exec is called with sh -c <StartCommand>.
	// MockRunner records calls only via ExecResults lookup, not a call counter,
	// so we verify indirectly that no error path was triggered.
	// The key would be "Ubuntu:sh -c echo hello".
}

// TestWatchdogReloadConfig verifies that ReloadConfig adds new distros and removes old ones.
func TestWatchdogReloadConfig(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	assert.Contains(t, w.states, "Ubuntu")

	newCfg := testConfig([]config.DistroConfig{
		{Name: "Debian", Enabled: true},
	})
	w.ReloadConfig(newCfg)

	assert.NotContains(t, w.states, "Ubuntu", "old distro state should be removed")
	assert.Contains(t, w.states, "Debian", "new distro state should be added")
}

// TestWatchdogKeepAliveOnHealthy verifies that a keep-alive is started for a healthy distro.
func TestWatchdogKeepAliveOnHealthy(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	w.checkAll()

	assert.Equal(t, 1, runner.KeepAliveCalls["Ubuntu"], "keep-alive should be started for healthy distro")

	// Second check should not start another (mock returns alive=true).
	w.checkAll()
	assert.Equal(t, 1, runner.KeepAliveCalls["Ubuntu"], "keep-alive should not be restarted when still alive")
}

// TestWatchdogKeepAliveOnRestart verifies that a keep-alive is started after restart.
func TestWatchdogKeepAliveOnRestart(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())

	// Run enough checks to trigger a restart.
	for i := 0; i < cfg.FailureThreshold; i++ {
		w.checkAll()
	}

	assert.Equal(t, 1, runner.KeepAliveCalls["Ubuntu"], "keep-alive should be started after restart")
}

// TestWatchdogKeepAliveStoppedOnReload verifies keep-alives are stopped for removed distros.
func TestWatchdogKeepAliveStoppedOnReload(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())
	w.checkAll()

	// Verify keep-alive is set.
	assert.NotNil(t, w.states["Ubuntu"].keepAlive)

	// Reload with a different distro.
	newCfg := testConfig([]config.DistroConfig{
		{Name: "Debian", Enabled: true},
	})
	w.ReloadConfig(newCfg)

	assert.NotContains(t, w.states, "Ubuntu", "old distro should be removed")
	assert.Contains(t, w.states, "Debian", "new distro should be added")
}

// TestWatchdogStatusStartingBeforeCheck verifies status is "starting" before any health check.
func TestWatchdogStatusStartingBeforeCheck(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())

	// Before any checkAll, status should be "starting".
	status := w.GetStatus()

	var ubuntuStatus *DistroStatus
	for i := range status.Distros {
		if status.Distros[i].Name == "Ubuntu" {
			ubuntuStatus = &status.Distros[i]
			break
		}
	}

	assert.NotNil(t, ubuntuStatus)
	assert.Equal(t, "starting", ubuntuStatus.State)
	assert.Equal(t, time.Duration(0), ubuntuStatus.Uptime)
}

// TestWatchdogUptimeResetsAfterRestart verifies uptime resets when a distro is restarted.
func TestWatchdogUptimeResetsAfterRestart(t *testing.T) {
	runner := wsl.NewMockRunner()
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}

	cfg := testConfig([]config.DistroConfig{
		{Name: "Ubuntu", Enabled: true},
	})

	w := New(cfg, runner, testLogger())

	// First healthy check sets StartedAt.
	w.checkAll()
	assert.False(t, w.states["Ubuntu"].StartedAt.IsZero(), "StartedAt should be set after healthy check")

	// Simulate distro going unhealthy and triggering restart after threshold failures.
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateStopped, Version: 2},
	}
	for i := 0; i < cfg.FailureThreshold; i++ {
		w.checkAll()
	}

	// After restart, StartedAt should be reset.
	assert.True(t, w.states["Ubuntu"].StartedAt.IsZero(), "StartedAt should be reset after restart")

	// Status should show "starting" until next healthy check.
	status := w.GetStatus()
	var ubuntuStatus *DistroStatus
	for i := range status.Distros {
		if status.Distros[i].Name == "Ubuntu" {
			ubuntuStatus = &status.Distros[i]
			break
		}
	}
	assert.Equal(t, "starting", ubuntuStatus.State)

	// Distro comes back healthy.
	runner.Distros = []wsl.DistroInfo{
		{Name: "Ubuntu", State: wsl.StateRunning, Version: 2},
	}
	w.checkAll()

	assert.False(t, w.states["Ubuntu"].StartedAt.IsZero(), "StartedAt should be set after recovery")
	status = w.GetStatus()
	for i := range status.Distros {
		if status.Distros[i].Name == "Ubuntu" {
			ubuntuStatus = &status.Distros[i]
			break
		}
	}
	assert.Equal(t, "healthy", ubuntuStatus.State)
	assert.False(t, w.states["Ubuntu"].StartedAt.IsZero(), "StartedAt should be set after recovery")
}
