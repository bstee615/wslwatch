package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 15*time.Second, cfg.CheckInterval)
	assert.Equal(t, 8*time.Second, cfg.ProbeTimeout)
	assert.Equal(t, 3*time.Second, cfg.RestartDelay)
	assert.Equal(t, 60*time.Second, cfg.FailureWindow)
	assert.Equal(t, 5, cfg.FailureThreshold)
	assert.Equal(t, time.Duration(0), cfg.BackoffDuration)
	assert.Contains(t, cfg.IgnoredDistros, "docker-desktop")
	assert.Contains(t, cfg.IgnoredDistros, "docker-desktop-data")
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu-22.04", Enabled: true},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LogLevel = "trace"
	assert.ErrorContains(t, cfg.Validate(), "invalid log_level")
}

func TestValidate_InvalidCheckInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CheckInterval = 0
	assert.ErrorContains(t, cfg.Validate(), "check_interval must be positive")
}

func TestValidate_InvalidProbeTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProbeTimeout = -1
	assert.ErrorContains(t, cfg.Validate(), "probe_timeout must be positive")
}

func TestValidate_NegativeRestartDelay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RestartDelay = -1
	assert.ErrorContains(t, cfg.Validate(), "restart_delay must be non-negative")
}

func TestValidate_InvalidFailureWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureWindow = 0
	assert.ErrorContains(t, cfg.Validate(), "failure_window must be positive")
}

func TestValidate_InvalidFailureThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 0
	assert.ErrorContains(t, cfg.Validate(), "failure_threshold must be positive")
}

func TestValidate_NegativeBackoffDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BackoffDuration = -1
	assert.ErrorContains(t, cfg.Validate(), "backoff_duration must be non-negative")
}

func TestValidate_EmptyDistroName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{{Name: "", Enabled: true}}
	assert.ErrorContains(t, cfg.Validate(), "must not be empty")
}

func TestValidate_DuplicateDistroName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu", Enabled: true},
		{Name: "Ubuntu", Enabled: true},
	}
	assert.ErrorContains(t, cfg.Validate(), "duplicate distro name")
}

func TestValidate_NegativeMaxRestarts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu", MaxRestarts: -1},
	}
	assert.ErrorContains(t, cfg.Validate(), "max_restarts must be non-negative")
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu-22.04", Enabled: true, MaxRestarts: 10},
	}

	// Save
	err := Save(cfg, path)
	require.NoError(t, err)

	// Load
	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, cfg.LogLevel, loaded.LogLevel)
	assert.Equal(t, cfg.CheckInterval, loaded.CheckInterval)
	assert.Equal(t, cfg.ProbeTimeout, loaded.ProbeTimeout)
	assert.Len(t, loaded.Distros, 1)
	assert.Equal(t, "Ubuntu-22.04", loaded.Distros[0].Name)
	assert.Equal(t, 10, loaded.Distros[0].MaxRestarts)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	err := os.WriteFile(path, []byte("{{invalid"), 0o644)
	require.NoError(t, err)

	_, err = Load(path)
	assert.Error(t, err)
}

func TestLoad_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	err := os.WriteFile(path, []byte("log_level: bogus\n"), 0o644)
	require.NoError(t, err)

	_, err = Load(path)
	assert.ErrorContains(t, err, "invalid config")
}

func TestSetByKey_TopLevel(t *testing.T) {
	cfg := DefaultConfig()

	assert.NoError(t, cfg.SetByKey("log_level", "debug"))
	assert.Equal(t, "debug", cfg.LogLevel)

	assert.NoError(t, cfg.SetByKey("check_interval", "30s"))
	assert.Equal(t, 30*time.Second, cfg.CheckInterval)

	assert.NoError(t, cfg.SetByKey("failure_threshold", "3"))
	assert.Equal(t, 3, cfg.FailureThreshold)
}

func TestSetByKey_DistroField(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu-22.04", Enabled: true, MaxRestarts: 0},
	}

	assert.NoError(t, cfg.SetByKey("distros.Ubuntu-22.04.max_restarts", "5"))
	assert.Equal(t, 5, cfg.Distros[0].MaxRestarts)

	assert.NoError(t, cfg.SetByKey("distros.Ubuntu-22.04.pause", "true"))
	assert.True(t, cfg.Distros[0].Pause)
}

func TestSetByKey_UnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	assert.ErrorContains(t, cfg.SetByKey("nonexistent", "val"), "unknown config key")
}

func TestSetByKey_UnknownDistro(t *testing.T) {
	cfg := DefaultConfig()
	assert.ErrorContains(t, cfg.SetByKey("distros.NonExistent.max_restarts", "5"), "distro not found")
}

func TestSetByKey_InvalidDuration(t *testing.T) {
	cfg := DefaultConfig()
	assert.Error(t, cfg.SetByKey("check_interval", "notaduration"))
}

func TestIsIgnored(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.IsIgnored("docker-desktop"))
	assert.True(t, cfg.IsIgnored("Docker-Desktop"))
	assert.True(t, cfg.IsIgnored("docker-desktop-data"))
	assert.False(t, cfg.IsIgnored("Ubuntu-22.04"))
}

func TestFindDistro(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Distros = []DistroConfig{
		{Name: "Ubuntu-22.04", Enabled: true},
		{Name: "Ubuntu-20.04", Enabled: true},
	}

	d := cfg.FindDistro("Ubuntu-22.04")
	require.NotNil(t, d)
	assert.Equal(t, "Ubuntu-22.04", d.Name)

	assert.Nil(t, cfg.FindDistro("NonExistent"))
}
