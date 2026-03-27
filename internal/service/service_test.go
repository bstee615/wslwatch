package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testSetup(t *testing.T) (*watchdog.Watchdog, *config.Config, string) {
	t.Helper()

	cfg := &config.Config{
		LogLevel:         "debug",
		CheckInterval:    1 * time.Second,
		ProbeTimeout:     5 * time.Second,
		RestartDelay:     100 * time.Millisecond,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 5,
		Distros: []config.DistroConfig{
			{Name: "TestDistro", Enabled: true},
		},
		IgnoredDistros: []string{"docker-desktop"},
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	err := config.Save(cfg, cfgPath)
	require.NoError(t, err)

	mock := &wsl.MockRunner{
		Handler: func(args []string) (string, error) { return "", nil },
	}
	w := watchdog.New(cfg, mock, testLogger())

	return w, cfg, cfgPath
}

func TestHandleIPCRequest_Status(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{Cmd: "status"})
	assert.True(t, resp.OK)

	var ws watchdog.WatchdogStatus
	err := json.Unmarshal(resp.Data, &ws)
	require.NoError(t, err)
	assert.True(t, ws.Running)
	assert.Len(t, ws.Distros, 1)
}

func TestHandleIPCRequest_Pause(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{
		Cmd:    "pause",
		Distro: "TestDistro",
	})
	assert.True(t, resp.OK)

	// Check distro is paused
	ws := w.Status()
	for _, d := range ws.Distros {
		if d.Name == "TestDistro" {
			assert.True(t, d.Paused)
		}
	}
}

func TestHandleIPCRequest_Pause_NoDistro(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{Cmd: "pause"})
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Error, "distro name required")
}

func TestHandleIPCRequest_Resume(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	// First pause
	_ = HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{
		Cmd:    "pause",
		Distro: "TestDistro",
	})

	// Then resume
	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{
		Cmd:    "resume",
		Distro: "TestDistro",
	})
	assert.True(t, resp.OK)

	ws := w.Status()
	for _, d := range ws.Distros {
		if d.Name == "TestDistro" {
			assert.False(t, d.Paused)
		}
	}
}

func TestHandleIPCRequest_Reload(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	// Modify the config file to add a distro
	cfg.Distros = append(cfg.Distros, config.DistroConfig{
		Name:    "NewDistro",
		Enabled: true,
	})
	err := config.Save(cfg, cfgPath)
	require.NoError(t, err)

	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{Cmd: "reload"})
	assert.True(t, resp.OK)
}

func TestHandleIPCRequest_Unknown(t *testing.T) {
	w, cfg, cfgPath := testSetup(t)

	resp := HandleIPCRequest(w, cfg, cfgPath, testLogger(), ipc.Request{Cmd: "bogus"})
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Error, "unknown command")
}
